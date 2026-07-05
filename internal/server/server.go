package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/ai"
	"github.com/EvolutionAPI/evolution-go/internal/audit"
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
	db         *sql.DB
}

func New(cfg *config.Config, app *service.Application, logger *slog.Logger, db *sql.DB) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORS(cfg.Security.CORSAllowedOrigins...))
	router.Use(middleware.RequestLogging(logger))

	rateLimitStore := middleware.NewRateLimitStore(cfg.RateLimit.Backend)
	searchLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.SearchRateLimitPolicy(cfg.RateLimit.MessagesPerMinute))
	broadcastLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.BroadcastRateLimitPolicy(cfg.RateLimit.BroadcastPerHour))
	webhookLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.WebhookRateLimitPolicy(cfg.RateLimit.WebhookCallsPerMinute))
	messageLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.MessageRateLimitPolicy(cfg.RateLimit.MessagesPerMinute))
	loginLimiter := middleware.NewRateLimiter(rateLimitStore, middleware.LoginRateLimitPolicy(cfg.RateLimit.LoginPerMinute))

	// Role sets: read = any authenticated role; op = day-to-day operation
	// (sends, connects) without config rights; admin = configuration changes.
	readRoles := []string{auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent, auth.RoleOperator, auth.RoleViewer}
	opRoles := []string{auth.RoleOwner, auth.RoleAdmin, auth.RoleAgent, auth.RoleOperator}
	adminRoles := []string{auth.RoleOwner, auth.RoleAdmin}

	authHandler := auth.NewHandler(app.Auth)
	auditHandler := audit.NewHandler(app.Audit)
	aiHandler := ai.NewHandler(app.AI)
	tenantHandler := tenant.NewHandler(app.Tenants)
	instanceHandler := instance.NewHandler(app.Instances)
	crmHandler := crm.NewHandler(app.CRM)
	broadcastHandler := broadcast.NewHandler(app.Broadcast)
	webhookHandler := webhook.NewHandler(app.Webhooks, app.Instances)
	dashboardHandler := dashboard.NewHandler(app.Dashboard)
	tenantMiddleware := middleware.NewTenantMiddleware(app.Tenants)

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "alive"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "database handle unavailable"})
			return
		}
		pingCtx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "error": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// IP-based limiter: brute-force protection on credential endpoints.
	router.POST("/auth/login", loginLimiter.MiddlewareByIP(), authHandler.Login)
	router.POST("/auth/refresh", loginLimiter.MiddlewareByIP(), authHandler.Refresh)
	// Tenant creation is a platform-level operation and must never be public.
	// Gated behind PLATFORM_API_KEY (falls back to GLOBAL_API_KEY).
	router.POST("/tenant", middleware.PlatformGuard(cfg.Security.PlatformAPIKey), tenantHandler.Create)

	protected := router.Group("/")
	protected.Use(middleware.NewAuthMiddleware(app.Auth).Guard())
	protected.Use(tenantMiddleware.Resolve())
	protected.Use(middleware.Audit(app.Audit))
	{
		protected.GET("/audit", middleware.RequireRoles(adminRoles...), auditHandler.List)
		protected.GET("/auth/me", authHandler.Me)
		protected.POST("/auth/logout", authHandler.Logout)
		protected.GET("/dashboard/metrics", middleware.RequireRoles(readRoles...), dashboardHandler.Metrics)
		protected.GET("/tenant", middleware.RequireRoles(readRoles...), tenantHandler.Get)
		protected.GET("/ai/settings", middleware.RequireRoles(readRoles...), aiHandler.GetTenantSettings)
		protected.PUT("/ai/settings", middleware.RequireRoles(adminRoles...), aiHandler.ConfigureTenant)
		protected.GET("/ai/instances/:instanceID", middleware.RequireRoles(readRoles...), aiHandler.GetInstanceSettings)
		protected.PUT("/ai/instances/:instanceID", middleware.RequireRoles(adminRoles...), aiHandler.ConfigureInstance)
		protected.POST("/instance", middleware.RequireRoles(adminRoles...), instanceHandler.Create)
		protected.GET("/instance", middleware.RequireRoles(readRoles...), instanceHandler.List)
		protected.POST("/instance/setPresence/:instanceName", middleware.RequireRoles(opRoles...), instanceHandler.LegacySetPresence)
		protected.GET("/instance/:id", middleware.RequireRoles(readRoles...), instanceHandler.Get)
		protected.GET("/instance/:id/settings", middleware.RequireRoles(readRoles...), instanceHandler.Settings)
		protected.GET("/instance/:id/advanced-settings", middleware.RequireRoles(readRoles...), instanceHandler.GetAdvancedSettings)
		protected.PUT("/instance/:id/advanced-settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateAdvancedSettings)
		protected.POST("/instance/:id/messages/text", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.SendText)
		protected.GET("/instance/:id/messages/text/:jobID", middleware.RequireRoles(readRoles...), instanceHandler.SendTextJobStatus)
		protected.POST("/instance/:id/chats/search", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.SearchChats)
		protected.POST("/instance/:id/messages/search", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.SearchMessages)
		protected.POST("/instance/:id/messages/media", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.SendMediaMessage)
		protected.POST("/instance/:id/messages/audio", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.SendAudioMessage)
		protected.GET("/instance/:id/websocket", middleware.RequireRoles(readRoles...), instanceHandler.GetWebsocketConfig)
		protected.PUT("/instance/:id/websocket", middleware.RequireRoles(adminRoles...), instanceHandler.SetWebsocketConfig)
		protected.GET("/instance/:id/rabbitmq", middleware.RequireRoles(readRoles...), instanceHandler.GetRabbitMQConfig)
		protected.PUT("/instance/:id/rabbitmq", middleware.RequireRoles(adminRoles...), instanceHandler.SetRabbitMQConfig)
		protected.GET("/instance/:id/sqs", middleware.RequireRoles(readRoles...), instanceHandler.GetSQSConfig)
		protected.PUT("/instance/:id/sqs", middleware.RequireRoles(adminRoles...), instanceHandler.SetSQSConfig)
		protected.GET("/instance/:id/proxy", middleware.RequireRoles(readRoles...), instanceHandler.GetProxyConfig)
		protected.PUT("/instance/:id/proxy", middleware.RequireRoles(adminRoles...), instanceHandler.SetProxyConfig)
		protected.GET("/instance/:id/chatwoot", middleware.RequireRoles(readRoles...), instanceHandler.GetChatwootConfig)
		protected.PUT("/instance/:id/chatwoot", middleware.RequireRoles(adminRoles...), instanceHandler.SetChatwootConfig)
		protected.GET("/instance/:id/openai", middleware.RequireRoles(readRoles...), instanceHandler.ListOpenAIResources)
		protected.POST("/instance/:id/openai", middleware.RequireRoles(adminRoles...), instanceHandler.CreateOpenAIResource)
		protected.GET("/instance/:id/openai/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetOpenAISettings)
		protected.PUT("/instance/:id/openai/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateOpenAISettings)
		protected.POST("/instance/:id/openai/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeOpenAIStatus)
		protected.GET("/instance/:id/openai/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetOpenAIResource)
		protected.PUT("/instance/:id/openai/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateOpenAIResource)
		protected.DELETE("/instance/:id/openai/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteOpenAIResource)
		protected.GET("/instance/:id/openai/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListOpenAISessions)
		protected.GET("/instance/:id/typebot", middleware.RequireRoles(readRoles...), instanceHandler.ListTypebotResources)
		protected.POST("/instance/:id/typebot", middleware.RequireRoles(adminRoles...), instanceHandler.CreateTypebotResource)
		protected.GET("/instance/:id/typebot/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetTypebotSettings)
		protected.PUT("/instance/:id/typebot/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateTypebotSettings)
		protected.POST("/instance/:id/typebot/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeTypebotStatus)
		protected.GET("/instance/:id/typebot/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetTypebotResource)
		protected.PUT("/instance/:id/typebot/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateTypebotResource)
		protected.DELETE("/instance/:id/typebot/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteTypebotResource)
		protected.GET("/instance/:id/typebot/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListTypebotSessions)
		protected.GET("/instance/:id/dify", middleware.RequireRoles(readRoles...), instanceHandler.ListDifyResources)
		protected.POST("/instance/:id/dify", middleware.RequireRoles(adminRoles...), instanceHandler.CreateDifyResource)
		protected.GET("/instance/:id/dify/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetDifySettings)
		protected.PUT("/instance/:id/dify/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateDifySettings)
		protected.POST("/instance/:id/dify/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeDifyStatus)
		protected.GET("/instance/:id/dify/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetDifyResource)
		protected.PUT("/instance/:id/dify/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateDifyResource)
		protected.DELETE("/instance/:id/dify/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteDifyResource)
		protected.GET("/instance/:id/dify/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListDifySessions)
		protected.GET("/instance/:id/n8n", middleware.RequireRoles(readRoles...), instanceHandler.ListN8NResources)
		protected.POST("/instance/:id/n8n", middleware.RequireRoles(adminRoles...), instanceHandler.CreateN8NResource)
		protected.GET("/instance/:id/n8n/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetN8NSettings)
		protected.PUT("/instance/:id/n8n/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateN8NSettings)
		protected.POST("/instance/:id/n8n/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeN8NStatus)
		protected.GET("/instance/:id/n8n/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetN8NResource)
		protected.PUT("/instance/:id/n8n/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateN8NResource)
		protected.DELETE("/instance/:id/n8n/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteN8NResource)
		protected.GET("/instance/:id/n8n/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListN8NSessions)
		protected.GET("/instance/:id/evoai", middleware.RequireRoles(readRoles...), instanceHandler.ListEvoAIResources)
		protected.POST("/instance/:id/evoai", middleware.RequireRoles(adminRoles...), instanceHandler.CreateEvoAIResource)
		protected.GET("/instance/:id/evoai/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetEvoAISettings)
		protected.PUT("/instance/:id/evoai/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateEvoAISettings)
		protected.POST("/instance/:id/evoai/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeEvoAIStatus)
		protected.GET("/instance/:id/evoai/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetEvoAIResource)
		protected.PUT("/instance/:id/evoai/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateEvoAIResource)
		protected.DELETE("/instance/:id/evoai/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteEvoAIResource)
		protected.GET("/instance/:id/evoai/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListEvoAISessions)
		protected.GET("/instance/:id/evolutionBot", middleware.RequireRoles(readRoles...), instanceHandler.ListEvolutionBotResources)
		protected.POST("/instance/:id/evolutionBot", middleware.RequireRoles(adminRoles...), instanceHandler.CreateEvolutionBotResource)
		protected.GET("/instance/:id/evolutionBot/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetEvolutionBotSettings)
		protected.PUT("/instance/:id/evolutionBot/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateEvolutionBotSettings)
		protected.POST("/instance/:id/evolutionBot/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeEvolutionBotStatus)
		protected.GET("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetEvolutionBotResource)
		protected.PUT("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateEvolutionBotResource)
		protected.DELETE("/instance/:id/evolutionBot/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteEvolutionBotResource)
		protected.GET("/instance/:id/evolutionBot/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListEvolutionBotSessions)
		protected.GET("/instance/:id/flowise", middleware.RequireRoles(readRoles...), instanceHandler.ListFlowiseResources)
		protected.POST("/instance/:id/flowise", middleware.RequireRoles(adminRoles...), instanceHandler.CreateFlowiseResource)
		protected.GET("/instance/:id/flowise/settings", middleware.RequireRoles(readRoles...), instanceHandler.GetFlowiseSettings)
		protected.PUT("/instance/:id/flowise/settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateFlowiseSettings)
		protected.POST("/instance/:id/flowise/status", middleware.RequireRoles(adminRoles...), instanceHandler.ChangeFlowiseStatus)
		protected.GET("/instance/:id/flowise/:resourceId", middleware.RequireRoles(readRoles...), instanceHandler.GetFlowiseResource)
		protected.PUT("/instance/:id/flowise/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateFlowiseResource)
		protected.DELETE("/instance/:id/flowise/:resourceId", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteFlowiseResource)
		protected.GET("/instance/:id/flowise/:resourceId/sessions", middleware.RequireRoles(readRoles...), instanceHandler.ListFlowiseSessions)
		protected.GET("/instance/id/:instanceID", middleware.RequireRoles(readRoles...), instanceHandler.GetByID)
		protected.GET("/instance/id/:instanceID/advanced-settings", middleware.RequireRoles(readRoles...), instanceHandler.GetAdvancedSettings)
		protected.PUT("/instance/id/:instanceID/advanced-settings", middleware.RequireRoles(adminRoles...), instanceHandler.UpdateAdvancedSettings)
		protected.POST("/instance/id/:instanceID/messages/text", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.SendText)
		protected.GET("/instance/id/:instanceID/messages/text/:jobID", middleware.RequireRoles(readRoles...), instanceHandler.SendTextJobStatus)
		protected.POST("/instance/id/:instanceID/chats/search", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.SearchChats)
		protected.POST("/instance/id/:instanceID/messages/search", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.SearchMessages)
		protected.GET("/group/fetchAllGroups/:instanceName", middleware.RequireRoles(readRoles...), instanceHandler.LegacyFetchAllGroups)
		protected.GET("/v2/group/findGroup/:instanceName", middleware.RequireRoles(readRoles...), instanceHandler.LegacyFindGroup)
		protected.GET("/v2/group/fetchAllGroups/:instanceName", middleware.RequireRoles(readRoles...), instanceHandler.LegacyFetchAllGroups)
		protected.POST("/chat/findChats/:instanceName", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.LegacyFindChats)
		protected.POST("/chat/findMessages/:instanceName", middleware.RequireRoles(readRoles...), searchLimiter.Middleware(), instanceHandler.LegacyFindMessages)
		protected.POST("/message/presence/:instanceName", middleware.RequireRoles(opRoles...), instanceHandler.LegacyChatPresence)
		protected.POST("/message/markread/:instanceName", middleware.RequireRoles(opRoles...), instanceHandler.LegacyMarkRead)
		protected.POST("/message/sendText/:instanceName", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.LegacySendText)
		protected.POST("/message/sendMedia/:instanceName", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.LegacySendMedia)
		protected.POST("/message/sendWhatsAppAudio/:instanceName", middleware.RequireRoles(opRoles...), messageLimiter.Middleware(), instanceHandler.LegacySendAudio)
		protected.POST("/instance/:id/connect", middleware.RequireRoles(adminRoles...), instanceHandler.Connect)
		protected.POST("/instance/id/:instanceID/connect", middleware.RequireRoles(adminRoles...), instanceHandler.ConnectByID)
		protected.POST("/instance/:id/disconnect", middleware.RequireRoles(adminRoles...), instanceHandler.Disconnect)
		protected.POST("/instance/id/:instanceID/disconnect", middleware.RequireRoles(adminRoles...), instanceHandler.DisconnectByID)
		protected.POST("/instance/:id/reconnect", middleware.RequireRoles(adminRoles...), instanceHandler.Reconnect)
		protected.POST("/instance/id/:instanceID/reconnect", middleware.RequireRoles(adminRoles...), instanceHandler.ReconnectByID)
		protected.POST("/instance/:id/pair", middleware.RequireRoles(adminRoles...), instanceHandler.Pair)
		protected.POST("/instance/id/:instanceID/pair", middleware.RequireRoles(adminRoles...), instanceHandler.PairByID)
		protected.DELETE("/instance/:id/logout", middleware.RequireRoles(adminRoles...), instanceHandler.Logout)
		protected.DELETE("/instance/id/:instanceID/logout", middleware.RequireRoles(adminRoles...), instanceHandler.LogoutByID)
		protected.GET("/instance/:id/status", middleware.RequireRoles(readRoles...), instanceHandler.Status)
		protected.GET("/instance/id/:instanceID/status", middleware.RequireRoles(readRoles...), instanceHandler.StatusByID)
		protected.GET("/instance/:id/runtime", middleware.RequireRoles(readRoles...), instanceHandler.RuntimeStatus)
		protected.GET("/instance/id/:instanceID/runtime", middleware.RequireRoles(readRoles...), instanceHandler.RuntimeStatusByID)
		protected.GET("/instance/:id/runtime/history", middleware.RequireRoles(readRoles...), instanceHandler.RuntimeHistory)
		protected.GET("/instance/id/:instanceID/runtime/history", middleware.RequireRoles(readRoles...), instanceHandler.RuntimeHistoryByID)
		protected.POST("/instance/:id/history/backfill", middleware.RequireRoles(adminRoles...), instanceHandler.BackfillHistory)
		protected.POST("/instance/id/:instanceID/history/backfill", middleware.RequireRoles(adminRoles...), instanceHandler.BackfillHistoryByID)
		protected.GET("/instance/:id/qr", middleware.RequireRoles(readRoles...), instanceHandler.QRCode)
		protected.GET("/instance/:id/qrcode", middleware.RequireRoles(readRoles...), instanceHandler.QRCode)
		protected.GET("/instance/id/:instanceID/qr", middleware.RequireRoles(readRoles...), instanceHandler.QRCodeByID)
		protected.GET("/instance/id/:instanceID/qrcode", middleware.RequireRoles(readRoles...), instanceHandler.QRCodeByID)
		protected.DELETE("/instance", middleware.RequireRoles(adminRoles...), instanceHandler.Delete)
		protected.DELETE("/instance/:id", middleware.RequireRoles(adminRoles...), instanceHandler.Delete)
		protected.DELETE("/instance/id/:instanceID", middleware.RequireRoles(adminRoles...), instanceHandler.DeleteByID)
		protected.GET("/contacts", middleware.RequireRoles(readRoles...), crmHandler.ListContacts)
		protected.GET("/contacts/:id", middleware.RequireRoles(readRoles...), crmHandler.GetContact)
		protected.POST("/contacts", middleware.RequireRoles(opRoles...), crmHandler.CreateContact)
		protected.PATCH("/contacts/:id", middleware.RequireRoles(opRoles...), crmHandler.UpdateContact)
		protected.POST("/contacts/:id/notes", middleware.RequireRoles(opRoles...), crmHandler.AddNote)
		protected.POST("/contacts/:id/tags", middleware.RequireRoles(opRoles...), crmHandler.AssignTags)
		protected.GET("/broadcast", middleware.RequireRoles(readRoles...), broadcastLimiter.Middleware(), broadcastHandler.List)
		protected.POST("/broadcast", middleware.RequireRoles(opRoles...), broadcastLimiter.Middleware(), broadcastHandler.Create)
		protected.GET("/broadcast/:id", middleware.RequireRoles(readRoles...), broadcastLimiter.Middleware(), broadcastHandler.Get)
		protected.GET("/broadcast/:id/recipients", middleware.RequireRoles(readRoles...), broadcastLimiter.Middleware(), broadcastHandler.ListRecipients)
		protected.GET("/webhook", middleware.RequireRoles(readRoles...), webhookHandler.List)
		protected.POST("/webhook", middleware.RequireRoles(adminRoles...), webhookHandler.Create)
		protected.GET("/webhook/deliveries", middleware.RequireRoles(readRoles...), webhookHandler.ListDeliveries)
		protected.GET("/webhook/:id", middleware.RequireRoles(readRoles...), webhookHandler.Get)
		protected.POST("/webhook/inbound", middleware.RequireRoles(opRoles...), webhookLimiter.Middleware(), webhookHandler.DispatchInbound)
		protected.POST("/webhook/outbound", middleware.RequireRoles(opRoles...), webhookLimiter.Middleware(), webhookHandler.DispatchOutbound)
	}

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.HTTP.Address,
			Handler:      router,
			ReadTimeout:  cfg.HTTP.ReadTimeout,
			WriteTimeout: cfg.HTTP.WriteTimeout,
		},
		logger: logger,
		db:     db,
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
