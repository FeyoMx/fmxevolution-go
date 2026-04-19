package broadcast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/instance"
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

func (m *broadcastRepoMock) CountByTenant(_ context.Context, tenantID string) (int64, error) {
	var total int64
	for _, job := range m.jobs {
		if job.TenantID == tenantID {
			total++
		}
	}
	return total, nil
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

func (instanceRepoMock) GetByGlobalID(_ context.Context, instanceID string) (*repository.Instance, error) {
	return &repository.Instance{ID: instanceID, TenantID: "tenant-1"}, nil
}

type contactRepoMock struct {
	contacts []repository.Contact
}

func (m contactRepoMock) ListContacts(_ context.Context, tenantID string) ([]repository.Contact, error) {
	items := make([]repository.Contact, 0, len(m.contacts))
	for _, contact := range m.contacts {
		if contact.TenantID == tenantID {
			items = append(items, contact)
		}
	}
	return items, nil
}

func (m contactRepoMock) CountContactsByTenant(_ context.Context, tenantID string) (int64, error) {
	var total int64
	for _, contact := range m.contacts {
		if contact.TenantID == tenantID {
			total++
		}
	}
	return total, nil
}

type senderMock struct {
	calls   []instance.SendTextInput
	errs    []error
	results []*instance.SendTextResult
}

func (m *senderMock) SendText(_ context.Context, tenantID, reference string, input instance.SendTextInput) (*instance.SendTextResult, *repository.Instance, error) {
	m.calls = append(m.calls, input)
	if len(m.results) > 0 {
		result := m.results[0]
		m.results = m.results[1:]
		if len(m.errs) > 0 {
			err := m.errs[0]
			m.errs = m.errs[1:]
			if err != nil {
				return nil, nil, err
			}
		}
		return result, &repository.Instance{ID: reference, TenantID: tenantID}, nil
	}
	if len(m.errs) > 0 {
		err := m.errs[0]
		m.errs = m.errs[1:]
		if err != nil {
			return nil, nil, err
		}
	}
	return &instance.SendTextResult{MessageID: "msg-1"}, &repository.Instance{ID: reference, TenantID: tenantID}, nil
}

type processorMock struct {
	err error
}

func (p processorMock) Process(context.Context, repository.BroadcastJob) error {
	return p.err
}

func TestCreateBroadcastJobSetsAvailability(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)

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

	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)
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

	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)
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

func TestDeliveryProcessorSendsToEligibleContacts(t *testing.T) {
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c2", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c3", TenantID: "tenant-1", Phone: "522222222222"},
		{ID: "c4", TenantID: "tenant-1", Phone: "523333333333", InstanceID: "instance-2"},
	}}
	sender := &senderMock{}
	processor := newDeliveryProcessor(instanceRepoMock{}, contacts, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:          "job-1",
		TenantID:    "tenant-1",
		InstanceID:  "instance-1",
		Message:     "hello",
		RatePerHour: 0,
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("expected 2 send attempts, got %d", len(sender.calls))
	}
	if sender.calls[0].Number != "521111111111" || sender.calls[1].Number != "522222222222" {
		t.Fatalf("unexpected recipients: %+v", sender.calls)
	}
}

func TestDeliveryProcessorFailsWithoutEligibleContacts(t *testing.T) {
	processor := newDeliveryProcessor(instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if isRetryableProcessorError(err) {
		t.Fatal("expected permanent failure when no eligible contacts exist")
	}
}

func TestDeliveryProcessorDoesNotRetryPartialDeliveryFailures(t *testing.T) {
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c2", TenantID: "tenant-1", Phone: "522222222222", InstanceID: "instance-1"},
	}}
	sender := &senderMock{errs: []error{nil, errors.New("runtime unavailable")}}
	processor := newDeliveryProcessor(instanceRepoMock{}, contacts, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if isRetryableProcessorError(err) {
		t.Fatal("expected partial delivery failure to be permanent")
	}
}

func TestDeliveryProcessorDoesNotTreatEmptySendResultAsSuccess(t *testing.T) {
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
	}}
	sender := &senderMock{results: []*instance.SendTextResult{{}}}
	processor := newDeliveryProcessor(instanceRepoMock{}, contacts, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !isRetryableProcessorError(err) {
		t.Fatal("expected empty send result without prior deliveries to remain retryable")
	}
}
