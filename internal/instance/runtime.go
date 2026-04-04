package instance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"

	pkgconfig "github.com/EvolutionAPI/evolution-go/pkg/config"
	producerInterfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	natsProducer "github.com/EvolutionAPI/evolution-go/pkg/events/nats"
	rabbitmqProducer "github.com/EvolutionAPI/evolution-go/pkg/events/rabbitmq"
	webhookProducer "github.com/EvolutionAPI/evolution-go/pkg/events/webhook"
	websocketProducer "github.com/EvolutionAPI/evolution-go/pkg/events/websocket"
	legacyInstanceModel "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	legacyInstanceRepo "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	legacyInstanceService "github.com/EvolutionAPI/evolution-go/pkg/instance/service"
	legacyLabelRepo "github.com/EvolutionAPI/evolution-go/pkg/label/repository"
	legacyLogger "github.com/EvolutionAPI/evolution-go/pkg/logger"
	legacyMessageRepo "github.com/EvolutionAPI/evolution-go/pkg/message/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	legacyWhatsmeow "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"

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
	return &LegacyRuntime{
		logger:        logger.With("module", "instance_runtime"),
		legacyCfg:     legacyCfg,
		legacyDB:      authDB,
		legacyRepo:    instanceRepository,
		legacySvc:     instanceService,
		whatsmeowSvc:  whatsmeowService,
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

	if _, _, _, err := r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
		WebhookUrl: instance.Webhook,
	}, instance); err != nil {
		return nil, err
	}

	return r.waitForActiveClient(instance.Id, clientReadyTimeout)
}

func (r *LegacyRuntime) refreshConnectedClient(instance *legacyInstanceModel.Instance) (*whatsmeow.Client, error) {
	client, err := r.waitForActiveClient(instance.Id, sendRetryDelay)
	if err == nil {
		return client, nil
	}

	if client, err = r.reconnectExistingClient(instance); err == nil {
		return client, nil
	}

	if _, _, _, connectErr := r.legacySvc.Connect(&legacyInstanceService.ConnectStruct{
		WebhookUrl: instance.Webhook,
	}, instance); connectErr != nil {
		if client, retryErr := r.waitForActiveClient(instance.Id, sendRetryDelay); retryErr == nil {
			return client, nil
		}
		return nil, connectErr
	}

	return r.waitForActiveClient(instance.Id, clientReadyTimeout)
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
