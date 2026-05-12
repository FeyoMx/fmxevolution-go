package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type recordingWebhookRepo struct {
	created []*repository.WebhookEndpoint
}

func (r *recordingWebhookRepo) Create(_ context.Context, endpoint *repository.WebhookEndpoint) error {
	r.created = append(r.created, endpoint)
	return nil
}

func (r *recordingWebhookRepo) GetByID(context.Context, string, string) (*repository.WebhookEndpoint, error) {
	return nil, errNotFound()
}

func (r *recordingWebhookRepo) ListByTenant(context.Context, string) ([]repository.WebhookEndpoint, error) {
	return nil, nil
}

type recordingLegacyInstanceService struct {
	calls []legacyWebhookCall
}

type legacyWebhookCall struct {
	tenantID   string
	reference  string
	webhookURL string
	events     []string
	base64     bool
	byEvents   bool
}

func (s *recordingLegacyInstanceService) Resolve(context.Context, string, string) (*repository.Instance, error) {
	return nil, errNotFound()
}

func (s *recordingLegacyInstanceService) SetWebhook(_ context.Context, tenantID, reference, webhookURL string, events []string, base64 bool, byEvents bool) (*repository.Instance, error) {
	s.calls = append(s.calls, legacyWebhookCall{
		tenantID:   tenantID,
		reference:  reference,
		webhookURL: webhookURL,
		events:     events,
		base64:     base64,
		byEvents:   byEvents,
	})
	return &repository.Instance{
		ID:              "instance-1",
		TenantID:        tenantID,
		Name:            reference,
		WebhookURL:      webhookURL,
		WebhookBase64:   base64,
		WebhookByEvents: byEvents,
	}, nil
}

func TestCreateWebhookEndpointDoesNotUseLegacyNameFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingWebhookRepo{}
	legacy := &recordingLegacyInstanceService{}
	handler := NewHandler(NewService(repo, nilLogger()), legacy)
	router := gin.New()
	router.POST("/webhook", func(c *gin.Context) {
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), domain.Identity{
			TenantID: "tenant-1",
			UserID:   "user-1",
			Role:     "owner",
		}))
		handler.Create(c)
	})

	body := map[string]any{
		"name":             "n8n endpoint",
		"url":              "https://hooks.example.test/inbound",
		"inbound_enabled":  true,
		"outbound_enabled": false,
		"signing_secret":   "secret",
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("expected endpoint create status 201, got %d body=%s", res.Code, res.Body.String())
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one endpoint to be created, got %d", len(repo.created))
	}
	if len(legacy.calls) != 0 {
		t.Fatalf("expected no legacy webhook update, got %+v", legacy.calls)
	}
	if repo.created[0].Name != "n8n endpoint" || repo.created[0].URL != "https://hooks.example.test/inbound" {
		t.Fatalf("unexpected endpoint: %+v", repo.created[0])
	}
}

func TestCreateWebhookLegacyNameFallbackRequiresLegacyShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &recordingWebhookRepo{}
	legacy := &recordingLegacyInstanceService{}
	handler := NewHandler(NewService(repo, nilLogger()), legacy)
	router := gin.New()
	router.POST("/webhook", func(c *gin.Context) {
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), domain.Identity{
			TenantID: "tenant-1",
			UserID:   "user-1",
			Role:     "owner",
		}))
		handler.Create(c)
	})

	body := map[string]any{
		"name": "AstethicBot",
		"webhook": map[string]any{
			"url":      "https://hooks.example.test/legacy",
			"enabled":  true,
			"base64":   true,
			"byEvents": true,
		},
		"events": []string{"MESSAGE"},
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected legacy update status 200, got %d body=%s", res.Code, res.Body.String())
	}
	if len(repo.created) != 0 {
		t.Fatalf("expected no endpoint create, got %d", len(repo.created))
	}
	if len(legacy.calls) != 1 {
		t.Fatalf("expected one legacy webhook update, got %d", len(legacy.calls))
	}
	call := legacy.calls[0]
	if call.reference != "AstethicBot" || call.webhookURL != "https://hooks.example.test/legacy" {
		t.Fatalf("unexpected legacy call: %+v", call)
	}
	if !call.base64 || !call.byEvents {
		t.Fatalf("expected legacy flags to be preserved, got %+v", call)
	}
}
