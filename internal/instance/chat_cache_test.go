package instance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type chatRuntimeMock struct {
	calls int
	items []chatSearchRecord
	err   error
}

func (m *chatRuntimeMock) SearchChats(context.Context, *repository.Instance, chatSearchFilter) ([]chatSearchRecord, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return cloneChatSearchRecords(m.items), nil
}

func (m *chatRuntimeMock) Connect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) Disconnect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) Reconnect(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) Logout(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) Pair(context.Context, *repository.Instance, string) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) RequestHistorySync(context.Context, *repository.Instance, HistoryBackfillRequest) (*HistoryBackfillResult, error) {
	return nil, nil
}

func (m *chatRuntimeMock) Snapshot(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func (m *chatRuntimeMock) QRCode(context.Context, *repository.Instance) (*RuntimeSnapshot, error) {
	return nil, nil
}

func TestSearchChatsUsesFreshCacheForRepeatedIdenticalQueries(t *testing.T) {
	runtime := &chatRuntimeMock{
		items: []chatSearchRecord{{ID: "chat-1", RemoteJID: "521@s.whatsapp.net", PushName: "Luis"}},
	}
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "alpha"}},
		nil,
		nil,
		runtime,
		nil,
		nil,
	)

	first, _, err := service.SearchChats(context.Background(), "tenant-1", "instance-1", ChatSearchRequest{Where: map[string]any{"query": "lu"}})
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	second, _, err := service.SearchChats(context.Background(), "tenant-1", "instance-1", ChatSearchRequest{Where: map[string]any{"query": "lu"}})
	if err != nil {
		t.Fatalf("second search: %v", err)
	}

	if runtime.calls != 1 {
		t.Fatalf("expected one live bridge call, got %d", runtime.calls)
	}
	if first.Meta.Source != "live" || first.Meta.Cached {
		t.Fatalf("expected first response to be live, got %+v", first.Meta)
	}
	if second.Meta.Source != "cache" || !second.Meta.Cached || second.Meta.Stale {
		t.Fatalf("expected second response to be fresh cache, got %+v", second.Meta)
	}
	if len(second.Items) != 1 || second.Items[0].RemoteJID != "521@s.whatsapp.net" {
		t.Fatalf("unexpected cached items: %+v", second.Items)
	}
}

func TestSearchChatsReturnsStaleCacheWhenBridgeFails(t *testing.T) {
	runtime := &chatRuntimeMock{
		items: []chatSearchRecord{{ID: "chat-1", RemoteJID: "521@s.whatsapp.net", PushName: "Luis"}},
	}
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "alpha"}},
		nil,
		nil,
		runtime,
		nil,
		nil,
	)
	service.chatCache = newChatSearchCache(time.Nanosecond, 5*time.Minute, time.Nanosecond)

	if _, _, err := service.SearchChats(context.Background(), "tenant-1", "instance-1", ChatSearchRequest{}); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	time.Sleep(time.Millisecond)
	runtime.err = errors.New("too many requests")

	result, _, err := service.SearchChats(context.Background(), "tenant-1", "instance-1", ChatSearchRequest{})
	if err != nil {
		t.Fatalf("stale fallback should not fail: %v", err)
	}
	if !result.Meta.Cached || !result.Meta.Stale || result.Meta.Reason != "bridge_rate_limited" {
		t.Fatalf("expected stale cache due rate limit, got %+v", result.Meta)
	}
}

func TestSearchChatsCacheIsTenantSafe(t *testing.T) {
	runtime := &chatRuntimeMock{
		items: []chatSearchRecord{{ID: "chat-1", RemoteJID: "521@s.whatsapp.net", PushName: "Luis"}},
	}
	service := NewService(
		lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-1", Name: "alpha"}},
		nil,
		nil,
		runtime,
		nil,
		nil,
	)

	if _, _, err := service.SearchChats(context.Background(), "tenant-1", "instance-1", ChatSearchRequest{}); err != nil {
		t.Fatalf("tenant-1 search: %v", err)
	}
	service.repo = lifecycleInstanceRepoMock{instance: &repository.Instance{ID: "instance-1", TenantID: "tenant-2", Name: "alpha"}}
	runtime.items = []chatSearchRecord{{ID: "chat-2", RemoteJID: "522@s.whatsapp.net", PushName: "Ana"}}
	result, _, err := service.SearchChats(context.Background(), "tenant-2", "instance-1", ChatSearchRequest{})
	if err != nil {
		t.Fatalf("tenant-2 search: %v", err)
	}

	if runtime.calls != 2 {
		t.Fatalf("expected separate live call for different tenant, got %d", runtime.calls)
	}
	if result.Meta.Cached {
		t.Fatalf("expected tenant-2 response to come from live bridge, got %+v", result.Meta)
	}
	if result.Items[0].RemoteJID != "522@s.whatsapp.net" {
		t.Fatalf("cache leaked across tenants: %+v", result.Items)
	}
}

func TestNormalizeChatSearchBridgeErrorMapsRateLimitToConflict(t *testing.T) {
	err := normalizeChatSearchBridgeError(errors.New("429 too many requests"))
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict for rate limit, got %v", err)
	}
}
