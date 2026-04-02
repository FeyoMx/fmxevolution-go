package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type broadcastRepoMock struct {
	jobs         map[string]*repository.BroadcastJob
	claimed      []repository.BroadcastJob
	markFailedAt *time.Time
	retryAt      *time.Time
}

func newBroadcastRepoMock() *broadcastRepoMock {
	return &broadcastRepoMock{jobs: make(map[string]*repository.BroadcastJob)}
}

func (m *broadcastRepoMock) Create(_ context.Context, job *repository.BroadcastJob) error {
	if job.ID == "" {
		job.ID = "job-1"
	}
	copied := *job
	m.jobs[job.ID] = &copied
	return nil
}

func (m *broadcastRepoMock) GetByID(_ context.Context, tenantID, jobID string) (*repository.BroadcastJob, error) {
	job := m.jobs[jobID]
	if job == nil || job.TenantID != tenantID {
		return nil, errors.New("record not found")
	}
	copied := *job
	return &copied, nil
}

func (m *broadcastRepoMock) ListByTenant(_ context.Context, tenantID string, _ int) ([]repository.BroadcastJob, error) {
	var jobs []repository.BroadcastJob
	for _, job := range m.jobs {
		if job.TenantID == tenantID {
			jobs = append(jobs, *job)
		}
	}
	return jobs, nil
}

func (m *broadcastRepoMock) ClaimNext(_ context.Context, workerID string, _ int, _ time.Time) ([]repository.BroadcastJob, error) {
	return append([]repository.BroadcastJob(nil), m.claimed...), nil
}

func (m *broadcastRepoMock) MarkCompleted(_ context.Context, tenantID, jobID string, completedAt time.Time) error {
	job := m.jobs[jobID]
	if job == nil || job.TenantID != tenantID {
		return errors.New("record not found")
	}
	job.Status = statusCompleted
	job.CompletedAt = &completedAt
	return nil
}

func (m *broadcastRepoMock) MarkFailed(_ context.Context, tenantID, jobID, message string, failedAt time.Time, retryAt *time.Time) error {
	job := m.jobs[jobID]
	if job == nil || job.TenantID != tenantID {
		return errors.New("record not found")
	}
	job.LastError = message
	m.markFailedAt = &failedAt
	m.retryAt = retryAt
	if retryAt != nil {
		job.Status = statusQueued
		job.AvailableAt = *retryAt
	} else {
		job.Status = statusFailed
		job.FailedAt = &failedAt
	}
	return nil
}

type instanceRepoMock struct{}

func (instanceRepoMock) GetByID(_ context.Context, tenantID, instanceID string) (*repository.Instance, error) {
	return &repository.Instance{ID: instanceID, TenantID: tenantID}, nil
}

type processorMock struct {
	err error
}

func (p processorMock) Process(context.Context, repository.BroadcastJob) error {
	return p.err
}

func TestCreateBroadcastJobSetsAvailability(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, nilLogger(), 1, 1)

	now := time.Now().UTC().Add(10 * time.Minute)
	job, err := service.Create(context.Background(), "tenant-1", CreateInput{
		InstanceID:  "instance-1",
		Message:     "hello",
		DelaySec:    30,
		ScheduledAt: &now,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if job.AvailableAt.Before(now) {
		t.Fatalf("expected available_at to respect scheduled time, got %v", job.AvailableAt)
	}
	if job.Status != statusQueued {
		t.Fatalf("expected queued status, got %s", job.Status)
	}
}

func TestHandleFailureReschedulesUntilMaxAttempts(t *testing.T) {
	repo := newBroadcastRepoMock()
	job := &repository.BroadcastJob{
		ID:          "job-1",
		TenantID:    "tenant-1",
		InstanceID:  "instance-1",
		Status:      statusProcessing,
		MaxAttempts: 3,
		Attempts:    1,
	}
	repo.jobs[job.ID] = job

	service := NewService(repo, instanceRepoMock{}, nilLogger(), 1, 1)
	service.processor = processorMock{err: errors.New("temporary failure")}

	service.handleJob(context.Background(), "worker-1", *job)

	updated := repo.jobs[job.ID]
	if updated.Status != statusQueued {
		t.Fatalf("expected queued retry status, got %s", updated.Status)
	}
	if repo.retryAt == nil {
		t.Fatal("expected retry_at to be set")
	}
}

func TestHandleFailureMarksPermanentFailure(t *testing.T) {
	repo := newBroadcastRepoMock()
	job := &repository.BroadcastJob{
		ID:          "job-2",
		TenantID:    "tenant-1",
		InstanceID:  "instance-1",
		Status:      statusProcessing,
		MaxAttempts: 1,
		Attempts:    1,
	}
	repo.jobs[job.ID] = job

	service := NewService(repo, instanceRepoMock{}, nilLogger(), 1, 1)
	service.processor = processorMock{err: errors.New("permanent failure")}

	service.handleJob(context.Background(), "worker-1", *job)

	updated := repo.jobs[job.ID]
	if updated.Status != statusFailed {
		t.Fatalf("expected failed status, got %s", updated.Status)
	}
	if updated.FailedAt == nil {
		t.Fatal("expected failed_at to be set")
	}
}
