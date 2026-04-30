package instance

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

const (
	defaultChatSearchFreshTTL     = 30 * time.Second
	defaultChatSearchStaleTTL     = 5 * time.Minute
	defaultChatSearchLiveThrottle = 5 * time.Second
)

type chatSearchRuntime interface {
	SearchChats(ctx context.Context, instance *repository.Instance, filter chatSearchFilter) ([]chatSearchRecord, error)
}

type ChatSearchResult struct {
	Items []chatSearchRecord
	Meta  ChatSearchMeta
}

type ChatSearchMeta struct {
	Cached          bool
	Stale           bool
	Source          string
	RefreshedAt     *time.Time
	ExpiresAt       *time.Time
	StaleUntil      *time.Time
	TTLSeconds      int
	StaleTTLSeconds int
	Reason          string
	OperatorMessage string
}

type chatSearchCache struct {
	mu              sync.Mutex
	entries         map[string]chatSearchCacheEntry
	freshTTL        time.Duration
	staleTTL        time.Duration
	liveThrottle    time.Duration
	lastLiveAttempt map[string]time.Time
}

type chatSearchCacheEntry struct {
	items       []chatSearchRecord
	refreshedAt time.Time
	expiresAt   time.Time
	staleUntil  time.Time
}

func newChatSearchCache(freshTTL, staleTTL, liveThrottle time.Duration) *chatSearchCache {
	if freshTTL <= 0 {
		freshTTL = defaultChatSearchFreshTTL
	}
	if staleTTL < freshTTL {
		staleTTL = freshTTL
	}
	if liveThrottle <= 0 {
		liveThrottle = defaultChatSearchLiveThrottle
	}
	return &chatSearchCache{
		entries:         make(map[string]chatSearchCacheEntry),
		freshTTL:        freshTTL,
		staleTTL:        staleTTL,
		liveThrottle:    liveThrottle,
		lastLiveAttempt: make(map[string]time.Time),
	}
}

func (c *chatSearchCache) fresh(tenantID, instanceID string, filter chatSearchFilter, now time.Time) (*ChatSearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := chatSearchCacheKey(tenantID, instanceID, filter)
	entry, ok := c.entries[key]
	if !ok || now.After(entry.expiresAt) {
		return nil, false
	}
	return c.resultFromEntry(entry, false, "cache", "fresh_cache"), true
}

func (c *chatSearchCache) throttled(tenantID, instanceID string, filter chatSearchFilter, now time.Time) (*ChatSearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := chatSearchCacheKey(tenantID, instanceID, filter)
	entry, ok := c.entries[key]
	if !ok || now.After(entry.staleUntil) {
		return nil, false
	}
	lastLiveAttempt := c.lastLiveAttempt[key]
	if lastLiveAttempt.IsZero() || now.Sub(lastLiveAttempt) >= c.liveThrottle {
		return nil, false
	}
	return c.resultFromEntry(entry, true, "cache", "live_query_throttled"), true
}

func (c *chatSearchCache) beginLiveAttempt(tenantID, instanceID string, filter chatSearchFilter, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := chatSearchCacheKey(tenantID, instanceID, filter)
	lastLiveAttempt := c.lastLiveAttempt[key]
	if !lastLiveAttempt.IsZero() && now.Sub(lastLiveAttempt) < c.liveThrottle {
		return false
	}
	c.lastLiveAttempt[key] = now
	return true
}

func (c *chatSearchCache) stale(tenantID, instanceID string, filter chatSearchFilter, now time.Time, reason string) (*ChatSearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := chatSearchCacheKey(tenantID, instanceID, filter)
	entry, ok := c.entries[key]
	if !ok || now.After(entry.staleUntil) {
		return nil, false
	}
	return c.resultFromEntry(entry, true, "cache", reason), true
}

func (c *chatSearchCache) store(tenantID, instanceID string, filter chatSearchFilter, items []chatSearchRecord, now time.Time) *ChatSearchResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := chatSearchCacheKey(tenantID, instanceID, filter)
	c.cleanupExpiredLocked(now)
	c.lastLiveAttempt[key] = now

	entry := chatSearchCacheEntry{
		items:       cloneChatSearchRecords(items),
		refreshedAt: now,
		expiresAt:   now.Add(c.freshTTL),
		staleUntil:  now.Add(c.staleTTL),
	}
	c.entries[key] = entry
	return c.resultFromEntry(entry, false, "live", "")
}

func (c *chatSearchCache) resultFromEntry(entry chatSearchCacheEntry, stale bool, source, reason string) *ChatSearchResult {
	refreshedAt := entry.refreshedAt
	expiresAt := entry.expiresAt
	staleUntil := entry.staleUntil
	cached := source == "cache"
	operatorMessage := "chat list returned from live bridge"
	if cached && stale {
		operatorMessage = "chat list returned from stale cache because live bridge refresh was not available"
	} else if cached {
		operatorMessage = "chat list returned from fresh cache"
	}
	return &ChatSearchResult{
		Items: cloneChatSearchRecords(entry.items),
		Meta: ChatSearchMeta{
			Cached:          cached,
			Stale:           stale,
			Source:          source,
			RefreshedAt:     &refreshedAt,
			ExpiresAt:       &expiresAt,
			StaleUntil:      &staleUntil,
			TTLSeconds:      int(c.freshTTL / time.Second),
			StaleTTLSeconds: int(c.staleTTL / time.Second),
			Reason:          reason,
			OperatorMessage: operatorMessage,
		},
	}
}

func (c *chatSearchCache) cleanupExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if now.After(entry.staleUntil) {
			delete(c.entries, key)
			delete(c.lastLiveAttempt, key)
		}
	}
}

func chatSearchCacheKey(tenantID, instanceID string, filter chatSearchFilter) string {
	return strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(instanceID),
		strings.ToLower(strings.TrimSpace(filter.RemoteJID)),
		strings.ToLower(strings.TrimSpace(filter.Query)),
	}, "\x00")
}

func cloneChatSearchRecords(items []chatSearchRecord) []chatSearchRecord {
	if len(items) == 0 {
		return []chatSearchRecord{}
	}
	cloned := make([]chatSearchRecord, len(items))
	copy(cloned, items)
	for idx := range cloned {
		if items[idx].Labels != nil {
			cloned[idx].Labels = append([]string(nil), items[idx].Labels...)
		}
	}
	return cloned
}
