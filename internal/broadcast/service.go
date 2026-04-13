package broadcast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/instance"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

const (
	statusQueued     = "queued"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"
)

type instanceFinder interface {
	GetByID(ctx context.Context, tenantID, instanceID string) (*repository.Instance, error)
}

type contactLister interface {
	ListContacts(ctx context.Context, tenantID string) ([]repository.Contact, error)
}

type textSender interface {
	SendText(ctx context.Context, tenantID, reference string, input instance.SendTextInput) (*instance.SendTextResult, *repository.Instance, error)
}

type processor interface {
	Process(ctx context.Context, job repository.BroadcastJob) error
}

type Service struct {
	repo           repository.BroadcastRepository
	instances      instanceFinder
	contacts       contactLister
	logger         *slog.Logger
	processor      processor
	workers        int
	claimBatchSize int
	dispatchEvery  time.Duration
	queue          chan repository.BroadcastJob
	once           sync.Once

	limiterMu      sync.Mutex
	nextInstanceAt map[string]time.Time
}

type CreateInput struct {
	InstanceID  string     `json:"instance_id"`
	Message     string     `json:"message"`
	RatePerHour int        `json:"rate_per_hour"`
	DelaySec    int        `json:"delay_sec"`
	MaxAttempts int        `json:"max_attempts"`
	ScheduledAt *time.Time `json:"scheduled_at"`
}

func NewService(repo repository.BroadcastRepository, instances instanceFinder, contacts contactLister, sender textSender, logger *slog.Logger, workers, claimBatchSize int) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if workers <= 0 {
		workers = 1
	}
	if claimBatchSize <= 0 {
		claimBatchSize = workers
	}

	return &Service{
		repo:           repo,
		instances:      instances,
		contacts:       contacts,
		logger:         logger,
		processor:      newDeliveryProcessor(instances, contacts, sender, logger),
		workers:        workers,
		claimBatchSize: claimBatchSize,
		dispatchEvery:  2 * time.Second,
		queue:          make(chan repository.BroadcastJob, workers*4),
		nextInstanceAt: make(map[string]time.Time),
	}
}

func (s *Service) Start(ctx context.Context) {
	s.once.Do(func() {
		for i := 0; i < s.workers; i++ {
			go s.worker(ctx, i+1)
		}
		go s.dispatcher(ctx)
	})
}

func (s *Service) Create(ctx context.Context, tenantID string, input CreateInput) (*repository.BroadcastJob, error) {
	input.InstanceID = strings.TrimSpace(input.InstanceID)
	input.Message = strings.TrimSpace(input.Message)
	if input.InstanceID == "" || input.Message == "" {
		return nil, fmt.Errorf("%w: instance_id and message are required", domain.ErrValidation)
	}
	if input.DelaySec < 0 {
		return nil, fmt.Errorf("%w: delay_sec cannot be negative", domain.ErrValidation)
	}
	if input.RatePerHour < 0 {
		return nil, fmt.Errorf("%w: rate_per_hour cannot be negative", domain.ErrValidation)
	}
	if input.MaxAttempts < 0 {
		return nil, fmt.Errorf("%w: max_attempts cannot be negative", domain.ErrValidation)
	}

	instance, err := s.instances.GetByID(ctx, tenantID, input.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("%w: instance not found for tenant", domain.ErrForbidden)
	}
	if instance == nil {
		return nil, fmt.Errorf("%w: instance not found for tenant", domain.ErrForbidden)
	}

	now := time.Now().UTC()
	if input.ScheduledAt != nil {
		scheduledAt := input.ScheduledAt.UTC()
		if scheduledAt.Before(now) {
			return nil, fmt.Errorf("%w: scheduled_at must be in the future", domain.ErrValidation)
		}
		input.ScheduledAt = &scheduledAt
	}
	availableAt := now
	if input.DelaySec > 0 {
		availableAt = availableAt.Add(time.Duration(input.DelaySec) * time.Second)
	}
	if input.ScheduledAt != nil && input.ScheduledAt.After(availableAt) {
		availableAt = input.ScheduledAt.UTC()
	}

	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	job := &repository.BroadcastJob{
		TenantID:    tenantID,
		InstanceID:  input.InstanceID,
		Message:     input.Message,
		RatePerHour: input.RatePerHour,
		DelaySec:    input.DelaySec,
		MaxAttempts: maxAttempts,
		ScheduledAt: input.ScheduledAt,
		AvailableAt: availableAt,
		Status:      statusQueued,
	}

	if err := s.repo.Create(ctx, job); err != nil {
		return nil, err
	}
	s.logger.Info("broadcast job queued", "tenant_id", tenantID, "instance_id", input.InstanceID, "message_length", len(input.Message), "available_at", availableAt)
	return job, nil
}

func (s *Service) Get(ctx context.Context, tenantID, jobID string) (*repository.BroadcastJob, error) {
	job, err := s.repo.GetByID(ctx, tenantID, jobID)
	if err != nil {
		return nil, fmt.Errorf("%w: broadcast job not found", domain.ErrNotFound)
	}
	return job, nil
}

func (s *Service) List(ctx context.Context, tenantID string, limit int) ([]repository.BroadcastJob, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	return s.repo.ListByTenant(ctx, tenantID, limit)
}

func (s *Service) dispatcher(ctx context.Context) {
	ticker := time.NewTicker(s.dispatchEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.claimAndEnqueue(ctx)
		}
	}
}

func (s *Service) claimAndEnqueue(ctx context.Context) {
	freeSlots := cap(s.queue) - len(s.queue)
	if freeSlots <= 0 {
		return
	}
	limit := s.claimBatchSize
	if freeSlots < limit {
		limit = freeSlots
	}

	workerID := "dispatcher"
	jobs, err := s.repo.ClaimNext(ctx, workerID, limit, time.Now().UTC())
	if err != nil {
		s.logger.Error("claim broadcast jobs", "error", err)
		return
	}

	for _, job := range jobs {
		s.tryEnqueue(job)
	}
}

func (s *Service) tryEnqueue(job repository.BroadcastJob) {
	select {
	case s.queue <- job:
	default:
		s.logger.Warn("broadcast queue buffer full", "job_id", job.ID, "tenant_id", job.TenantID)
	}
}

func (s *Service) worker(ctx context.Context, workerNumber int) {
	workerID := fmt.Sprintf("worker-%d", workerNumber)
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			s.handleJob(ctx, workerID, job)
		}
	}
}

func (s *Service) handleJob(ctx context.Context, workerID string, job repository.BroadcastJob) {
	s.waitForInstanceSlot(ctx, job.InstanceID, job.RatePerHour)

	processCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	if err := s.processor.Process(processCtx, job); err != nil {
		s.handleFailure(ctx, workerID, job, err)
		return
	}

	completedAt := time.Now().UTC()
	if err := s.repo.MarkCompleted(ctx, job.TenantID, job.ID, completedAt); err != nil {
		s.logger.Error("mark broadcast completed", "worker_id", workerID, "job_id", job.ID, "error", err)
		return
	}

	s.logger.Info("broadcast job completed", "worker_id", workerID, "job_id", job.ID, "tenant_id", job.TenantID, "instance_id", job.InstanceID)
}

func (s *Service) handleFailure(ctx context.Context, workerID string, job repository.BroadcastJob, err error) {
	now := time.Now().UTC()
	var retryAt *time.Time
	if job.Attempts < job.MaxAttempts && isRetryableProcessorError(err) {
		retry := now.Add(backoffForAttempt(job.Attempts))
		retryAt = &retry
	}

	if markErr := s.repo.MarkFailed(ctx, job.TenantID, job.ID, err.Error(), now, retryAt); markErr != nil {
		s.logger.Error("mark broadcast failed", "worker_id", workerID, "job_id", job.ID, "error", markErr)
		return
	}

	fields := []any{
		"worker_id", workerID,
		"job_id", job.ID,
		"tenant_id", job.TenantID,
		"instance_id", job.InstanceID,
		"error", err.Error(),
	}
	if retryAt != nil {
		fields = append(fields, "retry_at", retryAt.UTC())
		s.logger.Warn("broadcast job failed and rescheduled", fields...)
		return
	}

	s.logger.Error("broadcast job failed permanently", fields...)
}

type processorError struct {
	err       error
	retryable bool
}

func (e *processorError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *processorError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func retryableProcessorError(err error) error {
	if err == nil {
		return nil
	}
	return &processorError{err: err, retryable: true}
}

func permanentProcessorError(err error) error {
	if err == nil {
		return nil
	}
	return &processorError{err: err, retryable: false}
}

func isRetryableProcessorError(err error) bool {
	var typed *processorError
	if errors.As(err, &typed) {
		return typed.retryable
	}
	return true
}

func (s *Service) waitForInstanceSlot(ctx context.Context, instanceID string, ratePerHour int) {
	interval := intervalForRate(ratePerHour)

	for {
		s.limiterMu.Lock()
		now := time.Now().UTC()
		next := s.nextInstanceAt[instanceID]
		if next.IsZero() || !next.After(now) {
			s.nextInstanceAt[instanceID] = now.Add(interval)
			s.limiterMu.Unlock()
			return
		}
		wait := next.Sub(now)
		s.limiterMu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func intervalForRate(ratePerHour int) time.Duration {
	if ratePerHour <= 0 {
		return 500 * time.Millisecond
	}
	interval := time.Duration(int64(time.Hour) / int64(ratePerHour))
	if interval <= 0 {
		return 100 * time.Millisecond
	}
	return interval
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return 15 * time.Second
	}
	if attempt == 2 {
		return 1 * time.Minute
	}
	return 5 * time.Minute
}
