package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupPlatformRouter(key string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/tenant", PlatformGuard(key), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestPlatformGuard_ValidKey(t *testing.T) {
	r := setupPlatformRouter("super-secret-platform-key")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenant", nil)
	req.Header.Set("X-Platform-Key", "super-secret-platform-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid key, got %d", w.Code)
	}
}

func TestPlatformGuard_MissingKey(t *testing.T) {
	r := setupPlatformRouter("super-secret-platform-key")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenant", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", w.Code)
	}
}

func TestPlatformGuard_WrongKey(t *testing.T) {
	r := setupPlatformRouter("super-secret-platform-key")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenant", nil)
	req.Header.Set("X-Platform-Key", "wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", w.Code)
	}
}

func TestPlatformGuard_FailsClosedWhenUnconfigured(t *testing.T) {
	r := setupPlatformRouter("")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenant", nil)
	req.Header.Set("X-Platform-Key", "anything")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when platform key unconfigured, got %d", w.Code)
	}
}

func TestPlatformGuard_AcceptsApiKeyHeaderFallback(t *testing.T) {
	r := setupPlatformRouter("super-secret-platform-key")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenant", nil)
	req.Header.Set("X-API-Key", "super-secret-platform-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with X-API-Key fallback, got %d", w.Code)
	}
}
