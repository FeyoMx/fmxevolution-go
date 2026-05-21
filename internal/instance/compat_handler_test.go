package instance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type compatInstanceRepoMock struct {
	items []repository.Instance
}

func (m *compatInstanceRepoMock) Create(context.Context, *repository.Instance) error { return nil }

func (m *compatInstanceRepoMock) ListByTenant(_ context.Context, tenantID string) ([]repository.Instance, error) {
	items := make([]repository.Instance, 0, len(m.items))
	for _, item := range m.items {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (m *compatInstanceRepoMock) GetByID(_ context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	for idx := range m.items {
		if m.items[idx].TenantID == tenantID && m.items[idx].ID == instanceID {
			return &m.items[idx], nil
		}
	}
	return nil, errors.New("record not found")
}

func (m *compatInstanceRepoMock) GetByGlobalID(_ context.Context, instanceID string) (*repository.Instance, error) {
	for idx := range m.items {
		if m.items[idx].ID == instanceID {
			return &m.items[idx], nil
		}
	}
	return nil, errors.New("record not found")
}

func (m *compatInstanceRepoMock) FindByEngineInstanceID(_ context.Context, engineInstanceID string) (*repository.Instance, error) {
	for idx := range m.items {
		if m.items[idx].EngineInstanceID == engineInstanceID {
			return &m.items[idx], nil
		}
	}
	return nil, errors.New("record not found")
}

func (m *compatInstanceRepoMock) FindByName(_ context.Context, name string) (*repository.Instance, error) {
	for idx := range m.items {
		if strings.EqualFold(m.items[idx].Name, name) {
			return &m.items[idx], nil
		}
	}
	return nil, errors.New("record not found")
}

func (m *compatInstanceRepoMock) Update(_ context.Context, instance *repository.Instance) error {
	for idx := range m.items {
		if m.items[idx].ID == instance.ID {
			m.items[idx] = *instance
			return nil
		}
	}
	return errors.New("record not found")
}

func (m *compatInstanceRepoMock) Delete(context.Context, string, string) error { return nil }

func newCompatTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := NewService(&compatInstanceRepoMock{items: []repository.Instance{
		{
			ID:               "instance-1",
			TenantID:         "tenant-1",
			Name:             "ventas",
			EngineInstanceID: "engine-1",
			Status:           "open",
		},
	}}, nil, nil, nil, nil, nil)
	handler := NewHandler(service)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		identity := domain.Identity{TenantID: "tenant-1", Role: "admin"}
		c.Request = c.Request.WithContext(domain.WithIdentity(c.Request.Context(), identity))
		c.Next()
	})
	router.POST("/instance/setPresence/:instanceName", handler.LegacySetPresence)
	router.POST("/message/sendMedia/:instanceName", handler.LegacySendMedia)
	router.POST("/message/markread/:instanceName", handler.LegacyMarkRead)
	return router
}

func performCompatRequest(router http.Handler, path string, body string) (int, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	return rec.Code, payload
}

func TestLegacySetPresenceAvailable(t *testing.T) {
	code, payload := performCompatRequest(newCompatTestRouter(), "/instance/setPresence/ventas", `{"presence":"available"}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %#v", code, payload)
	}
	if payload["success"] != true {
		t.Fatalf("expected success=true, got %#v", payload)
	}
	data := payload["data"].(map[string]any)
	if data["alwaysOnline"] != true {
		t.Fatalf("expected alwaysOnline=true, got %#v", data)
	}
}

func TestLegacySetPresenceUnavailable(t *testing.T) {
	code, payload := performCompatRequest(newCompatTestRouter(), "/instance/setPresence/ventas", `{"state":"unavailable"}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %#v", code, payload)
	}
	data := payload["data"].(map[string]any)
	if data["alwaysOnline"] != false {
		t.Fatalf("expected alwaysOnline=false, got %#v", data)
	}
}

func TestLegacySendMediaWithCaptionReturnsCompatibleRuntimeError(t *testing.T) {
	code, payload := performCompatRequest(newCompatTestRouter(), "/message/sendMedia/ventas", `{
		"number":"5215512345678",
		"type":"image",
		"url":"https://example.com/image.jpg",
		"caption":"hello"
	}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("expected 500 without runtime, got %d: %#v", code, payload)
	}
	if payload["success"] != false {
		t.Fatalf("expected success=false, got %#v", payload)
	}
	if !strings.Contains(payload["error"].(string), "runtime unavailable") {
		t.Fatalf("expected runtime unavailable error, got %#v", payload)
	}
}

func TestLegacyMarkReadUnsupported(t *testing.T) {
	code, payload := performCompatRequest(newCompatTestRouter(), "/message/markread/ventas", `{"number":"5215512345678","id":["msg-1"]}`)
	if code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %#v", code, payload)
	}
	if payload["success"] != false || payload["error"] != "unsupported_markread" {
		t.Fatalf("expected unsupported_markread envelope, got %#v", payload)
	}
}

func TestLegacyCompatUnknownInstanceName(t *testing.T) {
	code, payload := performCompatRequest(newCompatTestRouter(), "/instance/setPresence/nope", `{"presence":"available"}`)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %#v", code, payload)
	}
	if payload["success"] != false {
		t.Fatalf("expected success=false, got %#v", payload)
	}
}
