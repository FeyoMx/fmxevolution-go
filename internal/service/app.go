package service

import (
	"context"
	"log/slog"

	"github.com/EvolutionAPI/evolution-go/internal/ai"
	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/broadcast"
	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/crm"
	"github.com/EvolutionAPI/evolution-go/internal/instance"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/EvolutionAPI/evolution-go/internal/tenant"
	"github.com/EvolutionAPI/evolution-go/internal/webhook"
)

type Application struct {
	Auth      *auth.Service
	Tenants   *tenant.Service
	Instances *instance.Service
	CRM       *crm.Service
	Broadcast *broadcast.Service
	Webhooks  *webhook.Service
	AI        *ai.Service
}

func NewApplication(stores *repository.Stores, cfg *config.Config, logger *slog.Logger) *Application {
	tokens := auth.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.TokenTTL, cfg.Auth.RefreshTTL)
	webhookService := webhook.NewService(stores.Webhooks, logger.With("module", "webhook"))
	aiService := ai.NewService(stores.AI, stores.Instances, &cfg.AI, logger.With("module", "ai"))
	aiService.SetOutboundDispatcher(aiWebhookDispatcher{service: webhookService})
	webhookService.SetAITrigger(webhookAITrigger{service: aiService})
	runtimeFactory := func() (instance.Runtime, error) {
		return instance.NewLegacyRuntime(logger)
	}
	var instanceRuntime instance.Runtime
	legacyRuntime, err := runtimeFactory()
	if err != nil {
		logger.Warn("legacy instance runtime unavailable; QR/connect real-time integration disabled", "error", err)
	} else {
		instanceRuntime = legacyRuntime
	}

	app := &Application{
		Auth:      auth.NewService(stores.Tenants, stores.Users, tokens),
		Tenants:   tenant.NewService(stores.Tenants, stores.Users),
		Instances: instance.NewService(stores.Instances, instanceRuntime, runtimeFactory, logger.With("module", "instance")),
		CRM:       crm.NewService(stores.CRM),
		Webhooks:  webhookService,
		AI:        aiService,
	}

	if instanceTokenResolver, err := auth.NewLegacyInstanceTokenResolver(stores.Instances); err != nil {
		logger.Warn("legacy instance token auth unavailable", "error", err)
	} else {
		app.Auth.SetInstanceTokenResolver(instanceTokenResolver)
	}

	app.Broadcast = broadcast.NewService(
		stores.Broadcasts,
		stores.Instances,
		logger.With("module", "broadcast"),
		cfg.Broadcast.Workers,
		cfg.Broadcast.QueueBatchSize,
	)

	return app
}

func (a *Application) Start(ctx context.Context) {
	a.Broadcast.Start(ctx)
	a.AI.Start(ctx)
}

type webhookAITrigger struct {
	service *ai.Service
}

func (w webhookAITrigger) HandleInboundAsync(ctx context.Context, tenantID string, input webhook.AITriggerInput) error {
	return w.service.HandleInboundAsync(ctx, tenantID, ai.IncomingMessageInput{
		EventType:       input.EventType,
		InstanceID:      input.InstanceID,
		ConversationKey: input.ConversationKey,
		MessageID:       input.MessageID,
		MessageText:     input.MessageText,
		Metadata:        input.Metadata,
	})
}

type aiWebhookDispatcher struct {
	service *webhook.Service
}

func (d aiWebhookDispatcher) DispatchOutbound(ctx context.Context, tenantID string, input ai.DispatchInput) ([]ai.DeliveryResult, error) {
	results, err := d.service.DispatchOutbound(ctx, tenantID, webhook.DispatchInput{
		EventType:  input.EventType,
		InstanceID: input.InstanceID,
		MessageID:  input.MessageID,
		Data:       input.Data,
	})
	if err != nil {
		return nil, err
	}

	converted := make([]ai.DeliveryResult, 0, len(results))
	for _, item := range results {
		converted = append(converted, ai.DeliveryResult{
			EndpointID:   item.EndpointID,
			EndpointName: item.EndpointName,
			URL:          item.URL,
			Delivered:    item.Delivered,
			StatusCode:   item.StatusCode,
			Error:        item.Error,
		})
	}

	return converted, nil
}
