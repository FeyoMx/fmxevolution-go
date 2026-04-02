package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/ai"
	"github.com/EvolutionAPI/evolution-go/internal/auth"
	"github.com/EvolutionAPI/evolution-go/internal/broadcast"
	"github.com/EvolutionAPI/evolution-go/internal/config"
	"github.com/EvolutionAPI/evolution-go/internal/crm"
	"github.com/EvolutionAPI/evolution-go/internal/dashboard"
	"github.com/EvolutionAPI/evolution-go/internal/instance"
	"github.com/EvolutionAPI/evolution-go/internal/middleware"
	"github.com/EvolutionAPI/evolution-go/internal/service"
	"github.com/EvolutionAPI/evolution-go/internal/tenant"
	"github.com/EvolutionAPI/evolution-go/internal/webhook"
	"github.com/gin-gonic/gin"
)

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

func New(cfg *config.Config, app *service.Application, logger *slog.Logger) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RequestLogging(logger))

	rateLimitStore := middleware.NewRateLimitStore(cfg.RateLimit.Backend)
	broadcastLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.BroadcastRateLimitPolicy(cfg.RateLimit.BroadcastPerHour))
	webhookLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.WebhookRateLimitPolicy(cfg.RateLimit.WebhookCallsPerMinute))

	authHandler := auth.NewHandler(app.Auth)
	aiHandler := ai.NewHandler(app.AI)
	tenantHandler := tenant.NewHandler(app.Tenants)
	instanceHandler := instance.NewHandler(app.Instances)
	crmHandler := crm.NewHandler(app.CRM)
	broadcastHandler := broadcast.NewHandler(app.Broadcast)
	webhookHandler := webhook.NewHandler(app.Webhooks, app.Instances)
	dashboardHandler := dashboard.NewHandler(instanceMetricsAdapter{service: app.Instances})
	tenantMiddleware := middleware.NewTenantMiddleware(app.Tenants)

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/tenant", tenantHandler.Create)

	protected := router.Group("/")
	protected.Use(middleware.NewAuthMiddleware(app.Auth).Guard())
	protected.Use(tenantMiddleware.Resolve())
	{
		protected.GET("/auth/me", authHandler.Me)
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/dashboard/metrics", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), dashboardHandler.Metrics)
		protected.GET("/tenant", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), tenantHandler.Get)
		protected.GET("/ai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), aiHandler.GetTenantSettings)
		protected.PUT("/ai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), aiHandler.ConfigureTenant)
		protected.GET("/ai/instances/:instanceID", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), aiHandler.GetInstanceSettings)
		protected.PUT("/ai/instances/:instanceID", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), aiHandler.ConfigureInstance)
		protected.POST("/instance", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.Create)
		protected.GET("/instance", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.List)
		protected.GET("/instance/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.Get)
		protected.GET("/instance/:id/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.Settings)
		protected.GET("/instance/:id/advanced-settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetAdvancedSettings)
		protected.PUT("/instance/:id/advanced-settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateAdvancedSettings)
		protected.POST("/instance/:id/messages/text", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SendText)
		protected.POST("/instance/:id/chats/search", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SearchChats)
		protected.POST("/instance/:id/messages/search", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SearchMessages)
		protected.POST("/instance/:id/messages/media", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SendMediaMessage)
		protected.POST("/instance/:id/messages/audio", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SendAudioMessage)
		protected.GET("/instance/:id/websocket", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetWebsocketConfig)
		protected.PUT("/instance/:id/websocket", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.SetWebsocketConfig)
		protected.GET("/instance/:id/rabbitmq", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetRabbitMQConfig)
		protected.PUT("/instance/:id/rabbitmq", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.SetRabbitMQConfig)
		protected.GET("/instance/:id/sqs", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetSQSConfig)
		protected.PUT("/instance/:id/sqs", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.SetSQSConfig)
		protected.GET("/instance/:id/proxy", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetProxyConfig)
		protected.PUT("/instance/:id/proxy", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.SetProxyConfig)
		protected.GET("/instance/:id/chatwoot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetChatwootConfig)
		protected.PUT("/instance/:id/chatwoot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.SetChatwootConfig)
		protected.GET("/instance/:id/openai", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListOpenAIResources)
		protected.POST("/instance/:id/openai", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateOpenAIResource)
		protected.GET("/instance/:id/openai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetOpenAISettings)
		protected.PUT("/instance/:id/openai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateOpenAISettings)
		protected.POST("/instance/:id/openai/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeOpenAIStatus)
		protected.GET("/instance/:id/openai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetOpenAIResource)
		protected.PUT("/instance/:id/openai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateOpenAIResource)
		protected.DELETE("/instance/:id/openai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteOpenAIResource)
		protected.GET("/instance/:id/openai/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListOpenAISessions)
		protected.GET("/instance/:id/typebot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListTypebotResources)
		protected.POST("/instance/:id/typebot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateTypebotResource)
		protected.GET("/instance/:id/typebot/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetTypebotSettings)
		protected.PUT("/instance/:id/typebot/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateTypebotSettings)
		protected.POST("/instance/:id/typebot/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeTypebotStatus)
		protected.GET("/instance/:id/typebot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetTypebotResource)
		protected.PUT("/instance/:id/typebot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateTypebotResource)
		protected.DELETE("/instance/:id/typebot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteTypebotResource)
		protected.GET("/instance/:id/typebot/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListTypebotSessions)
		protected.GET("/instance/:id/dify", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListDifyResources)
		protected.POST("/instance/:id/dify", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateDifyResource)
		protected.GET("/instance/:id/dify/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetDifySettings)
		protected.PUT("/instance/:id/dify/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateDifySettings)
		protected.POST("/instance/:id/dify/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeDifyStatus)
		protected.GET("/instance/:id/dify/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetDifyResource)
		protected.PUT("/instance/:id/dify/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateDifyResource)
		protected.DELETE("/instance/:id/dify/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteDifyResource)
		protected.GET("/instance/:id/dify/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListDifySessions)
		protected.GET("/instance/:id/n8n", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListN8NResources)
		protected.POST("/instance/:id/n8n", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateN8NResource)
		protected.GET("/instance/:id/n8n/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetN8NSettings)
		protected.PUT("/instance/:id/n8n/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateN8NSettings)
		protected.POST("/instance/:id/n8n/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeN8NStatus)
		protected.GET("/instance/:id/n8n/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetN8NResource)
		protected.PUT("/instance/:id/n8n/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateN8NResource)
		protected.DELETE("/instance/:id/n8n/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteN8NResource)
		protected.GET("/instance/:id/n8n/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListN8NSessions)
		protected.GET("/instance/:id/evoai", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListEvoAIResources)
		protected.POST("/instance/:id/evoai", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateEvoAIResource)
		protected.GET("/instance/:id/evoai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetEvoAISettings)
		protected.PUT("/instance/:id/evoai/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateEvoAISettings)
		protected.POST("/instance/:id/evoai/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeEvoAIStatus)
		protected.GET("/instance/:id/evoai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetEvoAIResource)
		protected.PUT("/instance/:id/evoai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateEvoAIResource)
		protected.DELETE("/instance/:id/evoai/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteEvoAIResource)
		protected.GET("/instance/:id/evoai/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListEvoAISessions)
		protected.GET("/instance/:id/evolutionBot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListEvolutionBotResources)
		protected.POST("/instance/:id/evolutionBot", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateEvolutionBotResource)
		protected.GET("/instance/:id/evolutionBot/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetEvolutionBotSettings)
		protected.PUT("/instance/:id/evolutionBot/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateEvolutionBotSettings)
		protected.POST("/instance/:id/evolutionBot/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeEvolutionBotStatus)
		protected.GET("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetEvolutionBotResource)
		protected.PUT("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateEvolutionBotResource)
		protected.DELETE("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteEvolutionBotResource)
		protected.GET("/instance/:id/evolutionBot/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListEvolutionBotSessions)
		protected.GET("/instance/:id/flowise", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListFlowiseResources)
		protected.POST("/instance/:id/flowise", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.CreateFlowiseResource)
		protected.GET("/instance/:id/flowise/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetFlowiseSettings)
		protected.PUT("/instance/:id/flowise/settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateFlowiseSettings)
		protected.POST("/instance/:id/flowise/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ChangeFlowiseStatus)
		protected.GET("/instance/:id/flowise/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetFlowiseResource)
		protected.PUT("/instance/:id/flowise/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateFlowiseResource)
		protected.DELETE("/instance/:id/flowise/:resourceId", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteFlowiseResource)
		protected.GET("/instance/:id/flowise/:resourceId/sessions", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.ListFlowiseSessions)
		protected.GET("/instance/id/:instanceID", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetByID)
		protected.GET("/instance/id/:instanceID/advanced-settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.GetAdvancedSettings)
		protected.PUT("/instance/id/:instanceID/advanced-settings", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.UpdateAdvancedSettings)
		protected.POST("/instance/id/:instanceID/messages/text", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.SendText)
		protected.POST("/instance/:id/connect", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.Connect)
		protected.POST("/instance/id/:instanceID/connect", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.ConnectByID)
		protected.POST("/instance/:id/disconnect", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.Disconnect)
		protected.POST("/instance/id/:instanceID/disconnect", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DisconnectByID)
		protected.GET("/instance/:id/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.Status)
		protected.GET("/instance/id/:instanceID/status", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.StatusByID)
		protected.GET("/instance/:id/qr", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.QRCode)
		protected.GET("/instance/:id/qrcode", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.QRCode)
		protected.GET("/instance/id/:instanceID/qr", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.QRCodeByID)
		protected.GET("/instance/id/:instanceID/qrcode", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), instanceHandler.QRCodeByID)
		protected.DELETE("/instance", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.Delete)
		protected.DELETE("/instance/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.Delete)
		protected.DELETE("/instance/id/:instanceID", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), instanceHandler.DeleteByID)
		protected.GET("/contacts", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.ListContacts)
		protected.GET("/contacts/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.GetContact)
		protected.POST("/contacts", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.CreateContact)
		protected.PATCH("/contacts/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.UpdateContact)
		protected.POST("/contacts/:id/notes", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.AddNote)
		protected.POST("/contacts/:id/tags", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), crmHandler.AssignTags)
		protected.GET("/broadcast", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), broadcastHandler.List)
		protected.POST("/broadcast", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), broadcastLimiter.Middleware(), broadcastHandler.Create)
		protected.GET("/broadcast/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), broadcastHandler.Get)
		protected.GET("/webhook", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), webhookHandler.List)
		protected.POST("/webhook", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin), webhookHandler.Create)
		protected.GET("/webhook/:id", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), webhookHandler.Get)
		protected.POST("/webhook/inbound", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), webhookLimiter.Middleware(), webhookHandler.DispatchInbound)
		protected.POST("/webhook/outbound", middleware.RequireRoles(auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent), webhookLimiter.Middleware(), webhookHandler.DispatchOutbound)
	}

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.HTTP.Address,
			Handler:      router,
			ReadTimeout:  cfg.HTTP.ReadTimeout,
			WriteTimeout: cfg.HTTP.WriteTimeout,
		},
		logger: logger,
	}
}

func (s *Server) Start() error {
	s.logger.Info("api server starting", "address", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

type instanceMetricsAdapter struct {
	service *instance.Service
}

func (a instanceMetricsAdapter) List(ctx context.Context, tenantID string) ([]dashboard.MetricInstance, error) {
	items, err := a.service.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	metrics := make([]dashboard.MetricInstance, 0, len(items))
	for _, item := range items {
		metrics = append(metrics, dashboard.MetricInstance{
			Status: item.Status,
		})
	}

	return metrics, nil
}
