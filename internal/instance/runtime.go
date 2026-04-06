package instance

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"

	pkgconfig "github.com/EvolutionAPI/evolution-go/pkg/config"
	producerInterfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	natsProducer "github.com/EvolutionAPI/evolution-go/pkg/events/nats"
	rabbitmqProducer "github.com/EvolutionAPI/evolution-go/pkg/events/rabbitmq"
	webhookProducer "github.com/EvolutionAPI/evolution-go/pkg/events/webhook"
	websocketProducer "github.com/EvolutionAPI/evolution-go/pkg/events/websocket"
	legacyGroupService "github.com/EvolutionAPI/evolution-go/pkg/group/service"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	legacyInstanceRepo "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	legacyInstanceService "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	legacyLabelRepo "github.com/EvolutionAPI/evolution-go/pkg/label/repository"
	legacyLogger "github.com/EvolutionAPI/evolution-go/pkg/logger"
	legacyMessageRepo "github.com/EvolutionAPI/evolution-go/pkg/message/repository"
	legacyUserService "github.com/EvolutionAPI/evolution-go/pkg/user/service"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	legacyWhatsmeow "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gabriel-vasile/mimetype"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type RuntimeSnapshot struct {
	Token        string
	Webhook      string
	Events       string
	JID          string
	ProfileName  string
	Connected    bool
	LoggedIn     bool
	Status       string
	QRCode       string
	PairingCode  string
	AlwaysOnline bool
	RejectCall   bool
	ReadMessages bool
	IgnoreGroups bool
	IgnoreStatus bool
}

type Runtime interface {
	Connect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Disconnect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Reconnect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Logout(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Pair(ctx context.Context, instance *repository.Instance, phone string) (*RuntimeSnapshot, error)
	Snapshot(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	QRCode(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
}

type SendTextResult struct {
	MessageID string    `json:"messageId"`
	ServerID  int64     `json:"serverId"`
	Chat      string    `json:"chat"`
	FromMe    bool      `json:"fromMe"`
	Timestamp time.Time `json:"timestamp"`
}

type SendMediaResult struct {
	MessageID   string    `json:"messageId"`
	ServerID    int64     `json:"serverId"`
	Chat        string    `json:"chat"`
	FromMe      bool      `json:"fromMe"`
	Timestamp   time.Time `json:"timestamp"`
	MessageType string    `json:"messageType"`
}

const (
	sendTextTimeout         = 45 * time.Second
	sendRetryDelay          = 3 * time.Second
	clientReadyTimeout      = 8 * time.Second
	clientReadyPollInterval = 500 * time.Millisecond
)

func (r *LegacyRuntime) GetAdvancedSettings(ctx context.Context, instance *repository.Instance) (*legacyInstanceModel.AdvancedSettings, error) {
	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	return r.legacySvc.GetAdvancedSettings(legacyInstance.Id)
}

func (r *LegacyRuntime) UpdateAdvancedSettings(ctx context.Context, instance *repository.Instance, settings *legacyInstanceModel.AdvancedSettings) (*RuntimeSnapshot, error) {
	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	if err := r.legacySvc.UpdateAdvancedSettings(legacyInstance.Id, settings); err != nil {
		return nil, err
	}

	return r.Snapshot(ctx, instance)
}

type LegacyRuntime struct {
	logger        *slog.Logger
	legacyCfg     *pkgconfig.Config
	legacyDB      *sql.DB
	legacyRepo    legacyInstanceRepo.InstanceRepository
	legacySvc     legacyInstanceService.InstanceService
	whatsmeowSvc  legacyWhatsmeow.WhatsmeowService
	groupSvc      legacyGroupService.GroupService
	userSvc       legacyUserService.UserService
	clientPointer map[string]*whatsmeow.Client
	sendLocks     sync.Map
	qrLocks       sync.Map
}

func NewLegacyRuntime(logger *slog.Logger) (*LegacyRuntime, error) {
	legacyCfg, err := loadLegacyConfig()
	if err != nil {
		return nil, err
	}

	usersDB, err := legacyCfg.CreateUsersDB()
	if err != nil {
		return nil, fmt.Errorf("open legacy users db: %w", err)
	}
	if err := usersDB.AutoMigrate(&legacyInstanceModel.Instance{}); err != nil {
		return nil, fmt.Errorf("migrate legacy instance schema: %w", err)
	}

	var authDB *sql.DB
	if legacyCfg.PostgresAuthDB != "" {
		authDB, err = openPostgresAuthDB(legacyCfg)
		if err != nil {
			return nil, err
		}
	}

	sqliteDB, exPath, err := initLegacyAuthDB(legacyCfg)
	if err != nil {
		return nil, err
	}

	killChannel := make(map[string]chan bool)
	clientPointer := make(map[string]*whatsmeow.Client)
	loggerManager := legacyLogger.NewLoggerManager(legacyCfg)

	instanceRepository := legacyInstanceRepo.NewInstanceRepository(usersDB)
	messageRepository := legacyMessageRepo.NewMessageRepository(usersDB)
	labelRepository := legacyLabelRepo.NewLabelRepository(usersDB)

	var rabbit producerInterfaces.Producer = rabbitmqProducer.NewRabbitMQProducer(nil, false, nil, nil, "", loggerManager)
	var webhook producerInterfaces.Producer = webhookProducer.NewWebhookProducer(legacyCfg.WebhookUrl, loggerManager)
	var websocket producerInterfaces.Producer = websocketProducer.NewWebsocketProducer(loggerManager)
	var nats producerInterfaces.Producer = noopProducer{}
	if strings.TrimSpace(legacyCfg.NatsUrl) != "" {
		nats = natsProducer.NewNatsProducer(legacyCfg.NatsUrl, legacyCfg.NatsGlobalEnabled, legacyCfg.NatsGlobalEvents, loggerManager)
	}

	whatsmeowService := legacyWhatsmeow.NewWhatsmeowService(
		instanceRepository,
		authDB,
		messageRepository,
		labelRepository,
		legacyCfg,
		killChannel,
		clientPointer,
		rabbit,
		webhook,
		websocket,
		sqliteDB,
		exPath,
		nil,
		nats,
		loggerManager,
	)

	instanceService := legacyInstanceService.NewInstanceService(
		instanceRepository,
		killChannel,
		clientPointer,
		whatsmeowService,
		legacyCfg,
		loggerManager,
	)
	groupService := legacyGroupService.NewGroupService(clientPointer, whatsmeowService, loggerManager)
	userService := legacyUserService.NewUserService(clientPointer, whatsmeowService, loggerManager)

	return &LegacyRuntime{
		logger:        logger.With("module", "instance_runtime"),
		legacyCfg:     legacyCfg,
		legacyDB:      authDB,
		legacyRepo:    instanceRepository,
		legacySvc:     instanceService,
		whatsmeowSvc:  whatsmeowService,
		groupSvc:      groupService,
		userSvc:       userService,
		clientPointer: clientPointer,
	}, nil
}

func (r *LegacyRuntime) SendText(ctx context.Context, instance *repository.Instance, number, text string) (*SendTextResult, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	sendLock := r.sendLock(legacyInstance.Id)
	sendLock.Lock()
	defer sendLock.Unlock()

	client, err := r.ensureConnectedClient(legacyInstance)
	if err != nil {
		return nil, err
	}

	recipient, err := r.resolveTextRecipient(client, legacyInstance, strings.TrimSpace(number))
	if err != nil {
		return nil, err
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &text,
		},
	}

	messageID := whatsmeow.GenerateMessageID()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if r.logger != nil {
				r.logger.Warn(
					"retrying send text after transient runtime failure",
					"instance_id", legacyInstance.Id,
					"number", strings.TrimSpace(number),
					"recipient", recipient.String(),
					"attempt", attempt+1,
					"previous_error", lastErr,
				)
			}
			time.Sleep(sendRetryDelay)

			client, err = r.refreshConnectedClient(legacyInstance)
			if err != nil {
				return nil, err
			}
		}

		sendCtx, cancel := context.WithTimeout(context.Background(), sendTextTimeout)
		response, err := client.SendMessage(sendCtx, recipient, msg, whatsmeow.SendRequestExtra{ID: messageID})
		cancel()
		if err == nil {
			return &SendTextResult{
				MessageID: messageID,
				ServerID:  int64(response.ServerID),
				Chat:      recipient.String(),
				FromMe:    true,
				Timestamp: time.Now(),
			}, nil
		}

		if errors.Is(sendCtx.Err(), context.DeadlineExceeded) {
			lastErr = fmt.Errorf("%w: send message timed out after %s", domain.ErrTimeout, sendTextTimeout)
			continue
		}

		lastErr = err
		if !isTransientSendError(err) {
			return nil, err
		}
	}

	return nil, lastErr
}

func (r *LegacyRuntime) SendMedia(ctx context.Context, instance *repository.Instance, input resolvedMediaMessageInput) (*SendMediaResult, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}
	sendLock := r.sendLock(legacyInstance.Id)
	sendLock.Lock()
	defer sendLock.Unlock()

	client, err := r.ensureConnectedClient(legacyInstance)
	if err != nil {
		return nil, err
	}

	recipient, err := r.resolveTextRecipient(client, legacyInstance, strings.TrimSpace(input.Number))
	if err != nil {
		return nil, err
	}

	fileData := input.FileData
	if len(fileData) == 0 && strings.TrimSpace(input.URL) != "" {
		fileData, err = downloadMediaURL(strings.TrimSpace(input.URL))
		if err != nil {
			return nil, err
		}
	}

	mediaMessage, uploadType, messageType, preparedData, err := r.buildOutgoingMediaMessage(input, fileData)
	if err != nil {
		return nil, err
	}

	sendCtx, cancel := context.WithTimeout(context.Background(), sendTextTimeout)
	defer cancel()

	uploaded, err := client.Upload(sendCtx, preparedData, uploadType)
	if err != nil {
		return nil, err
	}

	applyUploadToMediaMessage(mediaMessage, input, uploaded, uint64(len(preparedData)))
	messageID := whatsmeow.GenerateMessageID()
	response, err := client.SendMessage(sendCtx, recipient, mediaMessage, whatsmeow.SendRequestExtra{ID: messageID})
	if err != nil {
		return nil, err
	}

	return &SendMediaResult{
		MessageID:   messageID,
		ServerID:    int64(response.ServerID),
		Chat:        recipient.String(),
		FromMe:      true,
		Timestamp:   time.Now(),
		MessageType: messageType,
	}, nil
}

func (r *LegacyRuntime) SendAudio(ctx context.Context, instance *repository.Instance, input resolvedAudioMessageInput) (*SendMediaResult, error) {
	return r.SendMedia(ctx, instance, resolvedMediaMessageInput{
		Number:   input.Number,
		Type:     "audio",
		FileData: input.FileData,
		Delay:    input.Delay,
	})
}

func (r *LegacyRuntime) SearchChats(ctx context.Context, instance *repository.Instance, filter chatSearchFilter) ([]chatSearchRecord, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}
	if isNilInterface(r.userSvc) || isNilInterface(r.groupSvc) {
		return nil, fmt.Errorf("legacy chat services unavailable")
	}

	if _, err := r.ensureConnectedClient(legacyInstance); err != nil {
		return nil, err
	}

	contacts, contactErr := r.userSvc.GetContacts(legacyInstance)
	groups, groupErr := r.groupSvc.GetMyGroups(legacyInstance)
	if contactErr != nil && groupErr != nil {
		return nil, contactErr
	}

	records := make([]chatSearchRecord, 0, len(contacts)+len(groups))
	seen := make(map[string]struct{}, len(contacts)+len(groups))

	for _, contact := range contacts {
		remoteJID := strings.TrimSpace(contact.Jid)
		if remoteJID == "" || !contact.Found {
			continue
		}
		record := newChatSearchRecord(instance.ID, remoteJID, firstNonEmpty(contact.PushName, contact.FullName, contact.FirstName, contact.BusinessName, trimLegacyJID(remoteJID)))
		if !matchesChatFilter(record, filter) {
			continue
		}
		if _, ok := seen[record.RemoteJID]; ok {
			continue
		}
		seen[record.RemoteJID] = struct{}{}
		records = append(records, record)
	}

	for _, group := range groups {
		remoteJID := strings.TrimSpace(group.JID.String())
		if remoteJID == "" {
			continue
		}
		record := newChatSearchRecord(instance.ID, remoteJID, firstNonEmpty(group.GroupName.Name, trimLegacyJID(remoteJID)))
		if !matchesChatFilter(record, filter) {
			continue
		}
		if _, ok := seen[record.RemoteJID]; ok {
			continue
		}
		seen[record.RemoteJID] = struct{}{}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		left := strings.ToLower(records[i].PushName)
		right := strings.ToLower(records[j].PushName)
		if left == right {
			return records[i].RemoteJID < records[j].RemoteJID
		}
		return left < right
	})

	return records, nil
}

func (r *LegacyRuntime) Connect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	_, _, _, err = r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
		WebhookUrl: legacyInstance.Webhook,
	}, legacyInstance)
	if err != nil {
		return nil, err
	}

	snapshot, err := r.Snapshot(ctx, instance)
	if err != nil {
		return nil, err
	}
	if snapshot.Status == "" || snapshot.Status == "close" {
		snapshot.Status = "connecting"
	}
	if !snapshot.LoggedIn && snapshot.QRCode == "" {
		qr, qrErr := r.legacySvc.GetQr(legacyInstance)
		if qrErr == nil && qr != nil {
			snapshot.QRCode = qr.Qrcode
			snapshot.PairingCode = qr.Code
			snapshot.Status = "qrcode"
		}
	}
	return snapshot, nil
}

func (r *LegacyRuntime) Disconnect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	if _, err := r.legacySvc.Disconnect(legacyInstance); err != nil {
		return nil, err
	}

	return r.Snapshot(ctx, instance)
}

func (r *LegacyRuntime) Reconnect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	if err := r.whatsmeowSvc.ReconnectClient(legacyInstance.Id); err != nil {
		return nil, err
	}

	snapshot, err := r.Snapshot(ctx, instance)
	if err != nil {
		return nil, err
	}
	if snapshot.Status == "" || snapshot.Status == "close" {
		snapshot.Status = "connecting"
	}
	return snapshot, nil
}

func (r *LegacyRuntime) Logout(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	if _, err := r.legacySvc.Logout(legacyInstance); err != nil {
		return nil, err
	}

	snapshot, err := r.Snapshot(ctx, instance)
	if err != nil {
		snapshot = buildRuntimeSnapshot(legacyInstance, nil)
	}
	snapshot.Connected = false
	snapshot.LoggedIn = false
	snapshot.QRCode = ""
	snapshot.PairingCode = ""
	snapshot.Status = "close"
	return snapshot, nil
}

func (r *LegacyRuntime) Pair(ctx context.Context, instance *repository.Instance, phone string) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, fmt.Errorf("%w: phone is required", domain.ErrValidation)
	}

	client := r.clientPointer[legacyInstance.Id]
	if client != nil && client.IsLoggedIn() {
		return nil, fmt.Errorf("%w: pairing is only available while the runtime is awaiting login", domain.ErrConflict)
	}

	if client == nil || !client.IsConnected() {
		if _, _, _, err := r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
			WebhookUrl: legacyInstance.Webhook,
		}, legacyInstance); err != nil {
			return nil, err
		}
		client, err = r.waitForPairingClient(legacyInstance.Id, clientReadyTimeout)
		if err != nil {
			return nil, err
		}
	}

	code, err := client.PairPhone(context.Background(), phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return nil, err
	}

	snapshot := buildRuntimeSnapshot(legacyInstance, client)
	snapshot.PairingCode = code
	if snapshot.Status == "" || snapshot.Status == "close" {
		snapshot.Status = "connecting"
	}
	return snapshot, nil
}

func (r *LegacyRuntime) Snapshot(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	return buildRuntimeSnapshot(legacyInstance, r.clientPointer[legacyInstance.Id]), nil
}

func (r *LegacyRuntime) QRCode(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	qrLock := r.qrLock(legacyInstance.Id)
	qrLock.Lock()
	defer qrLock.Unlock()

	snapshot := buildRuntimeSnapshot(legacyInstance, r.clientPointer[legacyInstance.Id])
	// If the session is already active, do not re-enter the legacy QR flow.
	// Repeated /qrcode polling during an active session can contend with send/status work.
	if snapshot.LoggedIn || (snapshot.Connected && strings.EqualFold(snapshot.Status, "open")) {
		if snapshot.Status == "" || snapshot.Status == "close" {
			snapshot.Status = "open"
		}
		snapshot.QRCode = ""
		snapshot.PairingCode = ""
		return snapshot, nil
	}

	// If a client exists in memory but the websocket is currently down, avoid
	// hammering the legacy QR flow. There is no fresh QR to return yet.
	if r.hasClientPointer(legacyInstance.Id) && !snapshot.Connected && snapshot.QRCode == "" {
		snapshot.Status = "connecting"
		snapshot.PairingCode = ""
		return snapshot, nil
	}

	qr, err := r.legacySvc.GetQr(legacyInstance)
	if err != nil {
		if isQRCodePendingError(err) {
			snapshot.Status = "connecting"
			snapshot.QRCode = ""
			snapshot.PairingCode = ""
			return snapshot, nil
		}
		return nil, err
	}

	snapshot = buildRuntimeSnapshot(legacyInstance, r.clientPointer[legacyInstance.Id])
	snapshot.QRCode = qr.Qrcode
	snapshot.PairingCode = qr.Code
	if snapshot.Status == "" || snapshot.Status == "close" {
		snapshot.Status = "qrcode"
	}
	return snapshot, nil
}

func (r *LegacyRuntime) ensureLegacyInstance(_ context.Context, instance *repository.Instance) (*legacyInstanceModel.Instance, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyID := strings.TrimSpace(instance.EngineInstanceID)
	if legacyID == "" {
		legacyID = strings.TrimSpace(instance.ID)
	} else if _, err := uuid.Parse(legacyID); err != nil {
		legacyID = strings.TrimSpace(instance.ID)
	}

	legacyInstance, err := r.legacyRepo.GetInstanceByID(legacyID)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}

		legacyByName, nameErr := r.legacyRepo.GetInstanceByName(strings.TrimSpace(instance.Name))
		if nameErr == nil && legacyByName != nil {
			legacyInstance = legacyByName
		}
	}

	if legacyInstance == nil {
		created, createErr := r.legacySvc.Create(&legacyInstanceService.CreateStruct{
			InstanceId: legacyID,
			Name:       instance.Name,
			Token:      uuid.NewString(),
		})
		if createErr != nil {
			legacyByName, nameErr := r.legacyRepo.GetInstanceByName(strings.TrimSpace(instance.Name))
			if nameErr == nil && legacyByName != nil {
				legacyInstance = legacyByName
			} else {
				return nil, createErr
			}
		} else {
			legacyInstance = created
		}
	}

	if legacyInstance == nil {
		return nil, fmt.Errorf("legacy instance unavailable after sync")
	}

	changed := false
	if strings.TrimSpace(legacyInstance.Name) == "" || legacyInstance.Name != instance.Name {
		legacyInstance.Name = instance.Name
		changed = true
	}
	if strings.TrimSpace(legacyInstance.Token) == "" {
		legacyInstance.Token = uuid.NewString()
		changed = true
	}
	if strings.TrimSpace(legacyInstance.ClientName) == "" {
		legacyInstance.ClientName = r.legacyCfg.ClientName
		changed = true
	}
	if strings.TrimSpace(legacyInstance.Webhook) != strings.TrimSpace(instance.WebhookURL) {
		legacyInstance.Webhook = strings.TrimSpace(instance.WebhookURL)
		changed = true
	}
	if strings.TrimSpace(instance.WebhookURL) != "" {
		currentEvents := strings.TrimSpace(legacyInstance.Events)
		if currentEvents == "" || strings.EqualFold(currentEvents, "ALL") {
			legacyInstance.Events = "MESSAGE"
			changed = true
		}
	}
	if strings.TrimSpace(legacyInstance.OsName) == "" {
		legacyInstance.OsName = r.legacyCfg.OsName
		changed = true
	}
	if changed {
		if err := r.legacyRepo.Update(legacyInstance); err != nil {
			return nil, err
		}
	}

	return legacyInstance, nil
}

func buildRuntimeSnapshot(instance *legacyInstanceModel.Instance, client *whatsmeow.Client) *RuntimeSnapshot {
	snapshot := &RuntimeSnapshot{
		Token:        instance.Token,
		Webhook:      instance.Webhook,
		Events:       instance.Events,
		JID:          instance.Jid,
		AlwaysOnline: instance.AlwaysOnline,
		RejectCall:   instance.RejectCall,
		ReadMessages: instance.ReadMessages,
		IgnoreGroups: instance.IgnoreGroups,
		IgnoreStatus: instance.IgnoreStatus,
	}

	if parts := strings.Split(instance.Qrcode, "|"); len(parts) >= 2 {
		snapshot.QRCode = parts[0]
		snapshot.PairingCode = parts[1]
	}

	if client != nil {
		snapshot.Connected = client.IsConnected()
		snapshot.LoggedIn = client.IsLoggedIn()
		if client.Store != nil && client.Store.ID != nil {
			snapshot.JID = client.Store.ID.String()
		}
		if client.Store != nil && client.Store.PushName != "" {
			snapshot.ProfileName = client.Store.PushName
		}
	}

	switch {
	case snapshot.LoggedIn:
		snapshot.Status = "open"
	case snapshot.QRCode != "":
		snapshot.Status = "qrcode"
	case snapshot.Connected:
		snapshot.Status = "connecting"
	case instance.Connected:
		snapshot.Status = "open"
	default:
		snapshot.Status = "close"
	}

	return snapshot
}

func loadLegacyConfig() (_ *pkgconfig.Config, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("load legacy config: %v", recovered)
		}
	}()

	return pkgconfig.Load(), nil
}

func openPostgresAuthDB(cfg *pkgconfig.Config) (*sql.DB, error) {
	if err := cfg.EnsureDBExists(cfg.PostgresAuthDB); err != nil {
		return nil, fmt.Errorf("ensure legacy auth db: %w", err)
	}

	db, err := sql.Open("postgres", cfg.PostgresAuthDB)
	if err != nil {
		return nil, fmt.Errorf("open legacy auth db: %w", err)
	}

	return db, nil
}

func initLegacyAuthDB(cfg *pkgconfig.Config) (*sql.DB, string, error) {
	if cfg.PostgresAuthDB != "" {
		basePath, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		return nil, basePath, nil
	}

	basePath, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}

	dbDirectory := filepath.Join(basePath, "dbdata")
	if err := os.MkdirAll(dbDirectory, 0o751); err != nil {
		return nil, "", fmt.Errorf("create dbdata dir: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dbDirectory, "users.db")+"?_pragma=foreign_keys(1)&_busy_timeout=3000")
	if err != nil {
		return nil, "", err
	}

	return db, basePath, nil
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "record not found")
}

func (r *LegacyRuntime) ensureReady() error {
	if r == nil {
		return fmt.Errorf("legacy runtime unavailable")
	}
	if isNilInterface(r.legacyRepo) {
		return fmt.Errorf("legacy instance repository unavailable")
	}
	if isNilInterface(r.legacySvc) {
		return fmt.Errorf("legacy instance service unavailable")
	}
	if r.clientPointer == nil {
		return fmt.Errorf("legacy client runtime unavailable")
	}
	return nil
}

func (r *LegacyRuntime) ensureConnectedClient(instance *legacyInstanceModel.Instance) (*whatsmeow.Client, error) {
	if client, err := r.waitForActiveClient(instance.Id, 0); err == nil {
		return client, nil
	}

	if client, err := r.reconnectExistingClient(instance); err == nil {
		return client, nil
	}

	if client, err := r.hardReconnectClient(instance); err == nil {
		return client, nil
	}

	if _, _, _, err := r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
		WebhookUrl: instance.Webhook,
	}, instance); err != nil {
		if client, retryErr := r.hardReconnectClient(instance); retryErr == nil {
			return client, nil
		}
		return nil, err
	}

	if client, err := r.waitForActiveClient(instance.Id, clientReadyTimeout); err == nil {
		return client, nil
	}

	return r.hardReconnectClient(instance)
}

func (r *LegacyRuntime) refreshConnectedClient(instance *legacyInstanceModel.Instance) (*whatsmeow.Client, error) {
	client, err := r.waitForActiveClient(instance.Id, sendRetryDelay)
	if err == nil {
		return client, nil
	}

	if client, err = r.reconnectExistingClient(instance); err == nil {
		return client, nil
	}

	if client, err = r.hardReconnectClient(instance); err == nil {
		return client, nil
	}

	if _, _, _, connectErr := r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
		WebhookUrl: instance.Webhook,
	}, instance); connectErr != nil {
		if client, retryErr := r.waitForActiveClient(instance.Id, sendRetryDelay); retryErr == nil {
			return client, nil
		}
		if client, retryErr := r.hardReconnectClient(instance); retryErr == nil {
			return client, nil
		}
		return nil, connectErr
	}

	if client, err = r.waitForActiveClient(instance.Id, clientReadyTimeout); err == nil {
		return client, nil
	}

	return r.hardReconnectClient(instance)
}

func (r *LegacyRuntime) reconnectExistingClient(instance *legacyInstanceModel.Instance) (*whatsmeow.Client, error) {
	client := r.clientPointer[instance.Id]
	if client == nil {
		return nil, fmt.Errorf("client not initialized")
	}
	if client.IsConnected() && client.IsLoggedIn() {
		return client, nil
	}

	if err := r.legacySvc.Reconnect(instance); err != nil {
		return nil, err
	}

	return r.waitForActiveClient(instance.Id, clientReadyTimeout)
}

func (r *LegacyRuntime) hardReconnectClient(instance *legacyInstanceModel.Instance) (*whatsmeow.Client, error) {
	if isNilInterface(r.whatsmeowSvc) {
		return nil, fmt.Errorf("whatsmeow reconnect service unavailable")
	}

	if err := r.whatsmeowSvc.ReconnectClient(instance.Id); err != nil {
		return nil, err
	}

	return r.waitForActiveClient(instance.Id, clientReadyTimeout+sendRetryDelay)
}

func (r *LegacyRuntime) waitForActiveClient(instanceID string, timeout time.Duration) (*whatsmeow.Client, error) {
	deadline := time.Now().Add(timeout)
	for {
		client := r.clientPointer[instanceID]
		if client != nil && client.IsConnected() && client.IsLoggedIn() {
			return client, nil
		}
		if timeout == 0 || time.Now().After(deadline) {
			return nil, fmt.Errorf("no active session found")
		}
		time.Sleep(clientReadyPollInterval)
	}
}

func (r *LegacyRuntime) waitForPairingClient(instanceID string, timeout time.Duration) (*whatsmeow.Client, error) {
	deadline := time.Now().Add(timeout)
	for {
		client := r.clientPointer[instanceID]
		if client != nil && client.IsConnected() && !client.IsLoggedIn() {
			return client, nil
		}
		if timeout == 0 || time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: runtime did not become ready for pairing", domain.ErrTimeout)
		}
		time.Sleep(clientReadyPollInterval)
	}
}

func (r *LegacyRuntime) resolveTextRecipient(client *whatsmeow.Client, instance *legacyInstanceModel.Instance, number string) (types.JID, error) {
	formatJID := true
	recipient, err := validateLegacyMessageRecipient(number, &formatJID)
	if err != nil {
		return types.NewJID("", types.DefaultUserServer), err
	}

	if r.legacyCfg == nil || !r.legacyCfg.CheckUserExists {
		return recipient, nil
	}

	if strings.Contains(number, "@g.us") || strings.Contains(number, "@broadcast") || strings.Contains(number, "@newsletter") || strings.Contains(number, "@lid") {
		return recipient, nil
	}

	remoteJID, found, err := r.checkTextRecipientOnWhatsApp(client, number, false)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("recipient verification failed; using parsed jid", "instance_id", instance.Id, "number", number, "error", err)
		}
		return recipient, nil
	}
	if !found {
		remoteJID, found, err = r.checkTextRecipientOnWhatsApp(client, number, true)
		if err != nil {
			if r.logger != nil {
				r.logger.Warn("recipient verification retry failed; using parsed jid", "instance_id", instance.Id, "number", number, "error", err)
			}
			return recipient, nil
		}
	}
	if !found {
		return types.NewJID("", types.DefaultUserServer), fmt.Errorf("number %s is not registered on WhatsApp", number)
	}

	formatJID = false
	return validateLegacyMessageRecipient(remoteJID, &formatJID)
}

func validateLegacyMessageRecipient(phone string, formatJID *bool) (types.JID, error) {
	shouldFormat := true
	if formatJID != nil {
		shouldFormat = *formatJID
	}

	finalPhone := phone
	if shouldFormat {
		rawNumber := phone
		if strings.Contains(phone, "@s.whatsapp.net") {
			rawNumber = strings.Split(phone, "@")[0]
		}
		normalizedJID, err := utils.CreateJID(rawNumber)
		if err != nil {
			recipient, ok := utils.ParseJID(phone)
			if !ok {
				return types.NewJID("", types.DefaultUserServer), fmt.Errorf("could not parse phone: %s", phone)
			}
			finalPhone = recipient.String()
		} else {
			finalPhone = normalizedJID
		}
	}

	recipient, ok := utils.ParseJID(finalPhone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse formatted phone")
	}

	return recipient, nil
}

func (r *LegacyRuntime) checkTextRecipientOnWhatsApp(client *whatsmeow.Client, phone string, formatJID bool) (string, bool, error) {
	phoneNumbers, err := utils.PrepareNumbersForWhatsAppCheck([]string{phone}, &formatJID)
	if err != nil {
		return "", false, fmt.Errorf("failed to prepare number for WhatsApp check: %w", err)
	}

	resp, err := client.IsOnWhatsApp(context.Background(), phoneNumbers)
	if err != nil {
		return "", false, fmt.Errorf("failed to check if number %s exists on WhatsApp: %w", phoneNumbers[0], err)
	}
	if len(resp) == 0 {
		return "", false, nil
	}
	if !resp[0].IsIn {
		return "", false, nil
	}

	return resp[0].JID.String(), true, nil
}

func matchesChatFilter(record chatSearchRecord, filter chatSearchFilter) bool {
	if filter.RemoteJID != "" && !strings.EqualFold(strings.TrimSpace(record.RemoteJID), filter.RemoteJID) {
		return false
	}
	if filter.Query == "" {
		return true
	}

	query := strings.ToLower(strings.TrimSpace(filter.Query))
	return strings.Contains(strings.ToLower(record.PushName), query) ||
		strings.Contains(strings.ToLower(record.RemoteJID), query)
}

func trimLegacyJID(value string) string {
	return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "@s.whatsapp.net"), "@g.us")
}

func downloadMediaURL(rawURL string) ([]byte, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download media failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (r *LegacyRuntime) buildOutgoingMediaMessage(input resolvedMediaMessageInput, fileData []byte) (*waE2E.Message, whatsmeow.MediaType, string, []byte, error) {
	mime, _ := mimetype.DetectReader(bytes.NewReader(fileData))
	mimeType := mime.String()
	if strings.TrimSpace(input.MimeType) != "" {
		mimeType = strings.TrimSpace(input.MimeType)
	}

	switch input.Type {
	case "image":
		return &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:  proto.String(input.Caption),
				Mimetype: proto.String(mimeType),
			},
		}, whatsmeow.MediaImage, "ImageMessage", fileData, nil
	case "video":
		return &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				Caption:  proto.String(input.Caption),
				Mimetype: proto.String(mimeType),
			},
		}, whatsmeow.MediaVideo, "VideoMessage", fileData, nil
	case "audio":
		converted, duration, err := r.convertAudioPayload(fileData)
		if err != nil {
			return nil, whatsmeow.MediaAudio, "", nil, err
		}
		return &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				PTT:      proto.Bool(true),
				Mimetype: proto.String("audio/ogg; codecs=opus"),
				Seconds:  proto.Uint32(uint32(duration)),
			},
		}, whatsmeow.MediaAudio, "AudioMessage", converted, nil
	case "document":
		name := firstNonEmpty(input.FileName, "document")
		return &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{
				FileName: proto.String(name),
				Caption:  proto.String(input.Caption),
				Mimetype: proto.String(mimeType),
			},
		}, whatsmeow.MediaDocument, "DocumentMessage", fileData, nil
	default:
		return nil, whatsmeow.MediaDocument, "", nil, fmt.Errorf("%w: unsupported media type %q", domain.ErrValidation, input.Type)
	}
}

func applyUploadToMediaMessage(message *waE2E.Message, input resolvedMediaMessageInput, uploaded whatsmeow.UploadResponse, fileLength uint64) {
	switch {
	case message.GetImageMessage() != nil:
		msg := message.GetImageMessage()
		msg.URL = proto.String(uploaded.URL)
		msg.DirectPath = proto.String(uploaded.DirectPath)
		msg.MediaKey = uploaded.MediaKey
		msg.FileEncSHA256 = uploaded.FileEncSHA256
		msg.FileSHA256 = uploaded.FileSHA256
		msg.FileLength = proto.Uint64(fileLength)
	case message.GetVideoMessage() != nil:
		msg := message.GetVideoMessage()
		msg.URL = proto.String(uploaded.URL)
		msg.DirectPath = proto.String(uploaded.DirectPath)
		msg.MediaKey = uploaded.MediaKey
		msg.FileEncSHA256 = uploaded.FileEncSHA256
		msg.FileSHA256 = uploaded.FileSHA256
		msg.FileLength = proto.Uint64(fileLength)
	case message.GetAudioMessage() != nil:
		msg := message.GetAudioMessage()
		msg.URL = proto.String(uploaded.URL)
		msg.DirectPath = proto.String(uploaded.DirectPath)
		msg.MediaKey = uploaded.MediaKey
		msg.FileEncSHA256 = uploaded.FileEncSHA256
		msg.FileSHA256 = uploaded.FileSHA256
		msg.FileLength = proto.Uint64(uploaded.FileLength)
	case message.GetDocumentMessage() != nil:
		msg := message.GetDocumentMessage()
		msg.URL = proto.String(uploaded.URL)
		msg.DirectPath = proto.String(uploaded.DirectPath)
		msg.MediaKey = uploaded.MediaKey
		msg.FileEncSHA256 = uploaded.FileEncSHA256
		msg.FileSHA256 = uploaded.FileSHA256
		msg.FileLength = proto.Uint64(fileLength)
		if strings.TrimSpace(input.Caption) != "" {
			message.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{
				Message: &waE2E.Message{DocumentMessage: msg},
			}
			message.DocumentMessage = nil
		}
	}
}

type convertAudioRequest struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type convertAudioResponse struct {
	Duration int    `json:"duration"`
	Audio    string `json:"audio"`
}

func (r *LegacyRuntime) convertAudioPayload(fileData []byte) ([]byte, int, error) {
	if r.legacyCfg != nil && strings.TrimSpace(r.legacyCfg.ApiAudioConverter) != "" {
		return convertAudioWithAPI(r.legacyCfg.ApiAudioConverter, r.legacyCfg.ApiAudioConverterKey, convertAudioRequest{
			Base64: base64.StdEncoding.EncodeToString(fileData),
		})
	}
	return convertAudioToOpusWithDuration(fileData)
}

func convertAudioWithAPI(apiURL, apiKey string, payload convertAudioRequest) ([]byte, int, error) {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	if payload.URL != "" {
		if err := writer.WriteField("url", payload.URL); err != nil {
			return nil, 0, err
		}
	}
	if payload.Base64 != "" {
		if err := writer.WriteField("base64", payload.Base64); err != nil {
			return nil, 0, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, &requestBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("apikey", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("audio converter returned %d: %s", resp.StatusCode, string(body))
	}

	var output convertAudioResponse
	if err := json.Unmarshal(body, &output); err != nil {
		return nil, 0, err
	}

	decoded, err := base64.StdEncoding.DecodeString(output.Audio)
	if err != nil {
		return nil, 0, err
	}

	return decoded, output.Duration, nil
}

func convertAudioToOpusWithDuration(inputData []byte) ([]byte, int, error) {
	cmd := exec.Command("ffmpeg", "-i", "pipe:0",
		"-f", "ogg",
		"-vn",
		"-c:a", "libopus",
		"-avoid_negative_ts", "make_zero",
		"-b:a", "128k",
		"-ar", "48000",
		"-ac", "1",
		"-write_xing", "0",
		"-compression_level", "10",
		"-application", "voip",
		"-fflags", "+bitexact",
		"-flags", "+bitexact",
		"-id3v2_version", "0",
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-write_bext", "0",
		"pipe:1",
	)

	var outBuffer bytes.Buffer
	var errBuffer bytes.Buffer
	cmd.Stdin = bytes.NewReader(inputData)
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer

	if err := cmd.Run(); err != nil {
		return nil, 0, fmt.Errorf("audio conversion failed: %v: %s", err, errBuffer.String())
	}

	duration, err := extractFFmpegDuration(errBuffer.String())
	if err != nil {
		return nil, 0, err
	}

	return outBuffer.Bytes(), duration, nil
}

func extractFFmpegDuration(output string) (int, error) {
	re := regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) != 5 {
		return 0, errors.New("audio duration not found")
	}

	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])
	return hours*3600 + minutes*60 + seconds, nil
}

func isTransientSendError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "failed to get device list") ||
		strings.Contains(message, "failed to send usync query") ||
		strings.Contains(message, "context canceled")
}

func isQRCodePendingError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no qr code available") ||
		strings.Contains(message, "session already logged in")
}

func (r *LegacyRuntime) hasClientPointer(instanceID string) bool {
	_, ok := r.clientPointer[instanceID]
	return ok
}

func (r *LegacyRuntime) sendLock(instanceID string) *sync.Mutex {
	lock, _ := r.sendLocks.LoadOrStore(instanceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (r *LegacyRuntime) qrLock(instanceID string) *sync.Mutex {
	lock, _ := r.qrLocks.LoadOrStore(instanceID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}

	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

type noopProducer struct{}

func (noopProducer) Produce(string, []byte, string, string) error { return nil }

func (noopProducer) CreateGlobalQueues() error { return nil }
