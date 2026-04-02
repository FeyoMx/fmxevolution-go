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

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow"
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
	legacyWhatsmeow "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type RuntimeSnapshot struct {
	Token          string
	Webhook        string
	Events         string
	JID            string
	ProfileName    string
	Connected      bool
	LoggedIn       bool
	Status         string
	QRCode         string
	PairingCode    string
	AlwaysOnline   bool
	RejectCall     bool
	ReadMessages   bool
	IgnoreGroups   bool
	IgnoreStatus   bool
}

type Runtime interface {
	Connect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Disconnect(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	Snapshot(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
	QRCode(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error)
}

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
	clientPointer map[string]*whatsmeow.Client
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
		clientPointer: clientPointer,
	}, nil
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

	snapshot := buildRuntimeSnapshot(legacyInstance, r.clientPointer[legacyInstance.Id])
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

func (r *LegacyRuntime) QRCode(ctx context.Context, instance *repository.Instance) (*RuntimeSnapshot, error) {
	if err := r.ensureReady(); err != nil {
		return nil, err
	}

	legacyInstance, err := r.ensureLegacyInstance(ctx, instance)
	if err != nil {
		return nil, err
	}

	qr, err := r.legacySvc.GetQr(legacyInstance)
	if err != nil {
		return nil, err
	}

	snapshot := buildRuntimeSnapshot(legacyInstance, r.clientPointer[legacyInstance.Id])
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
		Token:         instance.Token,
		Webhook:       instance.Webhook,
		Events:        instance.Events,
		JID:           instance.Jid,
		AlwaysOnline:  instance.AlwaysOnline,
		RejectCall:    instance.RejectCall,
		ReadMessages:  instance.ReadMessages,
		IgnoreGroups:  instance.IgnoreGroups,
		IgnoreStatus:  instance.IgnoreStatus,
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
