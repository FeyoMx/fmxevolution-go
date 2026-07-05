package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
	"github.com/gin-gonic/gin"
)

type fakeRecorder struct {
	mu      sync.Mutex
	entries []repository.AuditLog
}

func (f *fakeRecorder) Record(entry repository.AuditLog) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
}

func (f *fakeRecorder) all() []repository.AuditLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]repository.AuditLog(nil), f.entries...)
}

func setupAuditRouter(rec *fakeRecorder, identity domain.Identity) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(identityKey, identity)
		c.Next()
	})
	r.Use(Audit(rec))
	handler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	r.POST("/instance/:id/messages/text", handler)
	r.GET("/instance", handler)
	r.DELETE("/instance/:id", handler)
	r.POST("/instance/:id/chats/search", handler)
	return r
}

func TestAudit_RecordsMessageSend(t *testing.T) {
	rec := &fakeRecorder{}
	r := setupAuditRouter(rec, domain.Identity{TenantID: "t1", UserID: "u1", Email: "a@b.co", Role: "admin"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/instance/abc/messages/text", nil)
	r.ServeHTTP(w, req)

	entries := rec.all()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Action != "message.send" || e.TenantID != "t1" || e.ActorType != "user" || e.ResourceID != "abc" || e.Status != http.StatusOK {
		t.Fatalf("unexpected entry: %+v", e)
	}
}

func TestAudit_SkipsReads(t *testing.T) {
	rec := &fakeRecorder{}
	r := setupAuditRouter(rec, domain.Identity{TenantID: "t1", UserID: "u1"})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instance", nil))

	// POST search endpoints are reads too — must be skipped.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/instance/abc/chats/search", nil))

	if entries := rec.all(); len(entries) != 0 {
		t.Fatalf("expected no audit entries for reads, got %d: %+v", len(entries), entries)
	}
}

func TestAudit_RecordsDeleteWithAPIKeyActor(t *testing.T) {
	rec := &fakeRecorder{}
	r := setupAuditRouter(rec, domain.Identity{TenantID: "t1", Role: "admin", APIKey: true})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/instance/abc", nil))

	entries := rec.all()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "instance.delete" || entries[0].ActorType != "api_key" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestAudit_SkipsWhenNoIdentity(t *testing.T) {
	rec := &fakeRecorder{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Audit(rec))
	r.POST("/instance", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/instance", nil))

	if entries := rec.all(); len(entries) != 0 {
		t.Fatalf("expected no entries without identity, got %d", len(entries))
	}
}

func TestRateLimiter_MiddlewareByIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewRateLimiter(NewMemoryRateLimitStore(), LoginRateLimitPolicy(2))
	r := gin.New()
	r.POST("/auth/login", limiter.MiddlewareByIP(), func(c *gin.Context) { c.Status(http.StatusOK) })

	codes := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		r.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}

	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("expected [200 200 429], got %v", codes)
	}
}
