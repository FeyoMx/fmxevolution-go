package audit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

// Service persists audit entries asynchronously so request latency is not
// affected by audit writes. Entries are queued on a buffered channel and
// written by a single worker; if the queue is full the entry is dropped with
// a warning rather than blocking the request path.
type Service struct {
	repo   repository.AuditRepository
	logger *slog.Logger
	queue  chan repository.AuditLog
	wg     sync.WaitGroup
	once   sync.Once
}

const queueSize = 1024

func NewService(repo repository.AuditRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:   repo,
		logger: logger,
		queue:  make(chan repository.AuditLog, queueSize),
	}
}

// Start launches the background writer. Safe to call once from app startup.
func (s *Service) Start(ctx context.Context) {
	s.once.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case entry, ok := <-s.queue:
					if !ok {
						return
					}
					s.write(entry)
				case <-ctx.Done():
					// Drain whatever is already queued before exiting.
					for {
						select {
						case entry := <-s.queue:
							s.write(entry)
						default:
							return
						}
					}
				}
			}
		}()
	})
}

// Stop waits briefly for the queue to drain during shutdown.
func (s *Service) Stop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Record enqueues an audit entry. Never blocks the caller.
func (s *Service) Record(entry repository.AuditLog) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	select {
	case s.queue <- entry:
	default:
		s.logger.Warn("audit queue full; dropping entry",
			"tenant_id", entry.TenantID, "action", entry.Action)
	}
}

// List returns audit entries for a tenant, newest first.
func (s *Service) List(ctx context.Context, tenantID string, filter repository.AuditLogFilter) ([]repository.AuditLog, error) {
	return s.repo.ListByTenant(ctx, tenantID, filter)
}

func (s *Service) write(entry repository.AuditLog) {
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.Create(writeCtx, &entry); err != nil {
		s.logger.Error("audit write failed",
			"tenant_id", entry.TenantID, "action", entry.Action, "error", err)
	}
}
