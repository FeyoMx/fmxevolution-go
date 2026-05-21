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
	"go.mau.fi/whatsmeow/types"
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

type compatMarkReadRuntime struct {
	input MarkReadInput
	err   error
	calls int
}

func (r *compatMarkReadRuntime) Connect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: true}, nil
}

func (r *compatMarkReadRuntime) Disconnect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: false}, nil
}

func (r *compatMarkReadRuntime) Reconnect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: true}, nil
}

func (r *compatMarkReadRuntime) Logout(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: false}, nil
}

func (r *compatMarkReadRuntime) Pair(context.Context, *repository.Instance, string) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: false}, nil
}

func (r *compatMarkReadRuntime) RequestHistorySync(context.Context, *repository.Instance, HistoryBackfillRequest) (*HistoryBackfillResult, error) {
	return &HistoryBackfillResult{Accepted: true}, nil
}

func (r *compatMarkReadRuntime) Snapshot(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{Connected: true}, nil
}

func (r *compatMarkReadRuntime) QRCode(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return &RuntimeSnapshot{}, nil
}

func (r *compatMarkReadRuntime) MarkRead(_ context.Context, _ *repository.Instance, input MarkReadInput) error {
	r.calls++
	r.input = input
	return r.err
}

func newCompatTestRouter() *gin.Engine {
	return newCompatTestRouterWithRuntime(nil)
}

func newCompatTestRouterWithRuntime(runtime Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := NewService(&compatInstanceRepoMock{items: []repository.Instance{
		{
			ID:               "instance-1",
			TenantID:         "tenant-1",
			Name:             "ventas",
			EngineInstanceID: "engine-1",
			Status:           "open",
		},
	}}, nil, nil, runtime, nil, nil)
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

func TestLegacyMarkReadDirectMessage(t *testing.T) {
	runtime := &compatMarkReadRuntime{}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{"number":"5215512345678","id":["ABCD123"]}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %#v", code, payload)
	}
	if payload["success"] != true || payload["message"] != "Messages marked as read" {
		t.Fatalf("expected markread success envelope, got %#v", payload)
	}
	data := payload["data"].(map[string]any)
	if data["count"].(float64) != 1 {
		t.Fatalf("expected count=1, got %#v", data)
	}
	if runtime.calls != 1 {
		t.Fatalf("expected one runtime call, got %d", runtime.calls)
	}
	if runtime.input.ChatJID.String() != "5215512345678@s.whatsapp.net" {
		t.Fatalf("unexpected chat jid: %s", runtime.input.ChatJID.String())
	}
	if !runtime.input.SenderJID.IsEmpty() {
		t.Fatalf("expected empty sender for DM, got %s", runtime.input.SenderJID.String())
	}
	if len(runtime.input.IDs) != 1 || runtime.input.IDs[0] != types.MessageID("ABCD123") {
		t.Fatalf("unexpected message IDs: %#v", runtime.input.IDs)
	}
}

func TestLegacyMarkReadGroupWithParticipant(t *testing.T) {
	runtime := &compatMarkReadRuntime{}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{
		"remoteJid":"120363123456789@g.us",
		"participant":"5215512345678@s.whatsapp.net",
		"id":["MSG1","MSG2"]
	}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %#v", code, payload)
	}
	if runtime.input.ChatJID.String() != "120363123456789@g.us" {
		t.Fatalf("unexpected group jid: %s", runtime.input.ChatJID.String())
	}
	if runtime.input.SenderJID.String() != "5215512345678@s.whatsapp.net" {
		t.Fatalf("unexpected participant jid: %s", runtime.input.SenderJID.String())
	}
	if len(runtime.input.IDs) != 2 {
		t.Fatalf("expected two message IDs, got %#v", runtime.input.IDs)
	}
}

func TestLegacyMarkReadPlayedReceipt(t *testing.T) {
	runtime := &compatMarkReadRuntime{}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{
		"key":{"id":"AUDIO1","remoteJid":"5215512345678@s.whatsapp.net","fromMe":false},
		"played":true
	}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %#v", code, payload)
	}
	if !runtime.input.Played {
		t.Fatalf("expected played receipt, got %#v", runtime.input)
	}
}

func TestLegacyMarkReadInvalidJID(t *testing.T) {
	runtime := &compatMarkReadRuntime{}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{"remoteJid":"not-a-whatsapp-jid@example.com","id":["MSG1"]}`)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %#v", code, payload)
	}
	if payload["success"] != false || payload["message"] != "Failed to mark messages as read" {
		t.Fatalf("expected markread error envelope, got %#v", payload)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime should not be called for invalid JID")
	}
}

func TestLegacyMarkReadOfflineInstance(t *testing.T) {
	runtime := &compatMarkReadRuntime{err: errors.New("client disconnected")}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{"number":"5215512345678","id":["MSG1"]}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %#v", code, payload)
	}
	if payload["success"] != false || payload["message"] != "Failed to mark messages as read" {
		t.Fatalf("expected markread error envelope, got %#v", payload)
	}
	if !strings.Contains(payload["error"].(string), "client disconnected") {
		t.Fatalf("expected disconnected error, got %#v", payload)
	}
}

func TestLegacyMarkReadEmptyMessageID(t *testing.T) {
	runtime := &compatMarkReadRuntime{}
	code, payload := performCompatRequest(newCompatTestRouterWithRuntime(runtime), "/message/markread/ventas", `{"number":"5215512345678","id":[""]}`)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %#v", code, payload)
	}
	if payload["success"] != false || payload["message"] != "Failed to mark messages as read" {
		t.Fatalf("expected markread error envelope, got %#v", payload)
	}
	if runtime.calls != 0 {
		t.Fatalf("runtime should not be called for empty message id")
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
