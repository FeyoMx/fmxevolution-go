package broadcast

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/EvolutionAPI/evolution-go/internal/instance"
	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type broadcastRepoMock struct {
	jobs              map[string]*repository.BroadcastJob
	claimed           []repository.BroadcastJob
	recipientProgress map[string]map[string]*repository.BroadcastRecipientProgress
	markFailedAt      *time.Time
	retryAt           *time.Time
}

func newBroadcastRepoMock() *broadcastRepoMock {
	return &broadcastRepoMock{
		jobs:              make(map[string]*repository.BroadcastJob),
		recipientProgress: make(map[string]map[string]*repository.BroadcastRecipientProgress),
	}
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

func (m *broadcastRepoMock) SeedRecipientProgress(_ context.Context, records []repository.BroadcastRecipientProgress) error {
	for _, record := range records {
		if record.ID == "" {
			record.ID = "progress-" + record.BroadcastID + "-" + record.Phone
		}
		if _, ok := m.recipientProgress[record.BroadcastID]; !ok {
			m.recipientProgress[record.BroadcastID] = make(map[string]*repository.BroadcastRecipientProgress)
		}
		if _, ok := m.recipientProgress[record.BroadcastID][record.Phone]; ok {
			continue
		}
		copied := record
		m.recipientProgress[record.BroadcastID][record.Phone] = &copied
	}
	return nil
}

func (m *broadcastRepoMock) SaveRecipientProgress(_ context.Context, progress *repository.BroadcastRecipientProgress) error {
	if progress == nil {
		return nil
	}
	if progress.ID == "" {
		progress.ID = "progress-" + progress.BroadcastID + "-" + progress.Phone
	}
	if _, ok := m.recipientProgress[progress.BroadcastID]; !ok {
		m.recipientProgress[progress.BroadcastID] = make(map[string]*repository.BroadcastRecipientProgress)
	}
	copied := *progress
	m.recipientProgress[progress.BroadcastID][progress.Phone] = &copied
	return nil
}

func (m *broadcastRepoMock) ListRecipientProgress(_ context.Context, tenantID, jobID string) ([]repository.BroadcastRecipientProgress, error) {
	items := make([]repository.BroadcastRecipientProgress, 0)
	for _, progress := range m.recipientProgress[jobID] {
		if progress.TenantID != tenantID {
			continue
		}
		items = append(items, *progress)
	}
	return items, nil
}

func (m *broadcastRepoMock) ListRecipientProgressPage(_ context.Context, tenantID, jobID string, filter repository.BroadcastRecipientProgressFilter) ([]repository.BroadcastRecipientProgress, int64, error) {
	items := make([]repository.BroadcastRecipientProgress, 0)
	for _, progress := range m.recipientProgress[jobID] {
		if progress.TenantID != tenantID {
			continue
		}
		if filter.Status != "" && progress.DeliveryStatus != filter.Status {
			continue
		}
		if filter.Query != "" {
			query := strings.ToLower(strings.TrimSpace(filter.Query))
			contactID := ""
			if progress.ContactID != nil {
				contactID = strings.ToLower(strings.TrimSpace(*progress.ContactID))
			}
			if !strings.Contains(strings.ToLower(progress.Phone), query) && !strings.Contains(contactID, query) {
				continue
			}
		}
		items = append(items, *progress)
	}

	total := int64(len(items))
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	start := (page - 1) * limit
	if start >= len(items) {
		return []repository.BroadcastRecipientProgress{}, total, nil
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

func (m *broadcastRepoMock) SummarizeRecipientProgress(_ context.Context, tenantID, jobID string) (repository.BroadcastRecipientAnalytics, error) {
	var summary repository.BroadcastRecipientAnalytics
	for _, progress := range m.recipientProgress[jobID] {
		if progress.TenantID != tenantID {
			continue
		}
		summary.TotalRecipients++
		if progress.AttemptCount > 0 {
			summary.Attempted++
		}
		switch progress.DeliveryStatus {
		case recipientStatusSent:
			summary.Sent++
		case recipientStatusDelivered:
			summary.Sent++
			summary.Delivered++
		case recipientStatusRead:
			summary.Sent++
			summary.Delivered++
			summary.Read++
		case recipientStatusFailed:
			summary.Failed++
		default:
			summary.Pending++
		}
	}
	if summary.TotalRecipients > 0 {
		summary.TrackedBroadcasts = 1
	}
	return summary, nil
}

func (m *broadcastRepoMock) SummarizeRecipientProgressByTenant(_ context.Context, tenantID string) (repository.BroadcastRecipientAnalytics, error) {
	var summary repository.BroadcastRecipientAnalytics
	for _, byPhone := range m.recipientProgress {
		tracked := false
		for _, progress := range byPhone {
			if progress.TenantID != tenantID {
				continue
			}
			tracked = true
			summary.TotalRecipients++
			if progress.AttemptCount > 0 {
				summary.Attempted++
			}
			switch progress.DeliveryStatus {
			case recipientStatusSent:
				summary.Sent++
			case recipientStatusDelivered:
				summary.Sent++
				summary.Delivered++
			case recipientStatusRead:
				summary.Sent++
				summary.Delivered++
				summary.Read++
			case recipientStatusFailed:
				summary.Failed++
			default:
				summary.Pending++
			}
		}
		if tracked {
			summary.TrackedBroadcasts++
		}
	}
	return summary, nil
}

func (m *broadcastRepoMock) MarkRecipientReceipt(_ context.Context, tenantID, instanceID, messageID, state string, at time.Time) (bool, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	for _, byPhone := range m.recipientProgress {
		for _, progress := range byPhone {
			if progress.TenantID != tenantID || progress.InstanceID != instanceID || strings.TrimSpace(progress.MessageID) != strings.TrimSpace(messageID) {
				continue
			}
			timestamp := at.UTC()
			switch state {
			case "delivered":
				progress.DeliveryStatus = recipientStatusDelivered
				progress.DeliveredAt = &timestamp
				progress.LastStatusAt = &timestamp
				progress.StatusSource = "receipt_delivered"
			case "read":
				progress.DeliveryStatus = recipientStatusRead
				progress.DeliveredAt = &timestamp
				progress.ReadAt = &timestamp
				progress.LastStatusAt = &timestamp
				progress.StatusSource = "receipt_read"
			default:
				return false, nil
			}
			return true, nil
		}
	}
	return false, nil
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

func (m *broadcastRepoMock) MarkCompletedWithFailures(_ context.Context, tenantID, jobID, message string, completedAt time.Time) error {
	job := m.jobs[jobID]
	if job == nil || job.TenantID != tenantID {
		return errors.New("record not found")
	}
	job.Status = statusCompletedWithFailures
	job.LastError = message
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

func TestCreateBroadcastJobEnrichesRecipientAnalytics(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)

	job := &repository.BroadcastJob{ID: "job-analytics", TenantID: "tenant-1", InstanceID: "instance-1", Status: statusQueued}
	repo.jobs[job.ID] = job
	_ = repo.SeedRecipientProgress(context.Background(), []repository.BroadcastRecipientProgress{
		{BroadcastID: job.ID, TenantID: "tenant-1", InstanceID: "instance-1", Phone: "1", DeliveryStatus: recipientStatusSent, AttemptCount: 1},
		{BroadcastID: job.ID, TenantID: "tenant-1", InstanceID: "instance-1", Phone: "2", DeliveryStatus: recipientStatusPending, AttemptCount: 0},
	})

	enriched, err := service.Get(context.Background(), "tenant-1", job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if enriched.RecipientSent != 1 || enriched.RecipientPending != 1 || len(enriched.Recipients) != 2 {
		t.Fatalf("unexpected recipient analytics: %+v", enriched)
	}
}

func TestListRecipientsReturnsPaginatedFilteredItems(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)

	job := &repository.BroadcastJob{ID: "job-list", TenantID: "tenant-1", InstanceID: "instance-1", Status: statusQueued}
	repo.jobs[job.ID] = job
	contactID := "contact-1"
	_ = repo.SeedRecipientProgress(context.Background(), []repository.BroadcastRecipientProgress{
		{BroadcastID: job.ID, TenantID: "tenant-1", InstanceID: "instance-1", ContactID: &contactID, Phone: "521111111111", DeliveryStatus: recipientStatusPending},
		{BroadcastID: job.ID, TenantID: "tenant-1", InstanceID: "instance-1", Phone: "522222222222", DeliveryStatus: recipientStatusSent, AttemptCount: 1},
		{BroadcastID: job.ID, TenantID: "tenant-1", InstanceID: "instance-1", Phone: "523333333333", DeliveryStatus: recipientStatusFailed, AttemptCount: 1, LastError: "bad number"},
	})

	result, err := service.ListRecipients(context.Background(), "tenant-1", job.ID, ListRecipientsInput{
		Page:   1,
		Limit:  1,
		Status: recipientStatusSent,
		Query:  "2222",
	})
	if err != nil {
		t.Fatalf("list recipients: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected paginated result: %+v", result)
	}
	if result.Items[0].Phone != "522222222222" || result.Summary.TotalRecipients != 3 {
		t.Fatalf("unexpected recipient payload: %+v", result)
	}
}

func TestListRecipientsRejectsUnsupportedStatusFilter(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)
	job := &repository.BroadcastJob{ID: "job-invalid", TenantID: "tenant-1", InstanceID: "instance-1", Status: statusQueued}
	repo.jobs[job.ID] = job

	_, err := service.ListRecipients(context.Background(), "tenant-1", job.ID, ListRecipientsInput{Status: "accepted"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestHandleReceiptUpdatesRecipientDeliveryProgress(t *testing.T) {
	repo := newBroadcastRepoMock()
	service := NewService(repo, instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger(), 1, 1)
	job := &repository.BroadcastJob{ID: "job-receipt", TenantID: "tenant-1", InstanceID: "instance-1", Status: statusCompleted}
	repo.jobs[job.ID] = job

	_ = repo.SeedRecipientProgress(context.Background(), []repository.BroadcastRecipientProgress{
		{
			BroadcastID:    job.ID,
			TenantID:       "tenant-1",
			InstanceID:     "instance-1",
			Phone:          "521111111111",
			DeliveryStatus: recipientStatusSent,
			AttemptCount:   1,
			MessageID:      "msg-1",
		},
	})

	deliveredAt := time.Now().UTC()
	if err := service.HandleReceipt(context.Background(), "instance-1", "msg-1", "delivered", deliveredAt); err != nil {
		t.Fatalf("handle delivered receipt: %v", err)
	}
	if err := service.HandleReceipt(context.Background(), "instance-1", "msg-1", "read", deliveredAt.Add(time.Minute)); err != nil {
		t.Fatalf("handle read receipt: %v", err)
	}

	progress, _ := repo.ListRecipientProgress(context.Background(), "tenant-1", job.ID)
	if len(progress) != 1 {
		t.Fatalf("expected 1 recipient progress row, got %d", len(progress))
	}
	if progress[0].DeliveryStatus != recipientStatusRead || progress[0].DeliveredAt == nil || progress[0].ReadAt == nil {
		t.Fatalf("expected read progression to be stored, got %+v", progress[0])
	}
	if progress[0].StatusSource != "receipt_read" || progress[0].LastStatusAt == nil {
		t.Fatalf("expected receipt metadata to be stored, got %+v", progress[0])
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
	processor := newDeliveryProcessor(newBroadcastRepoMock(), instanceRepoMock{}, contacts, sender, nilLogger())

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
	processor := newDeliveryProcessor(newBroadcastRepoMock(), instanceRepoMock{}, contactRepoMock{}, &senderMock{}, nilLogger())

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

func TestDeliveryProcessorPausesForRetryableFailureAfterPartialDelivery(t *testing.T) {
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c2", TenantID: "tenant-1", Phone: "522222222222", InstanceID: "instance-1"},
	}}
	repo := newBroadcastRepoMock()
	sender := &senderMock{errs: []error{nil, errors.New("runtime unavailable")}}
	processor := newDeliveryProcessor(repo, instanceRepoMock{}, contacts, sender, nilLogger())

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
		t.Fatal("expected partial delivery failure to remain retryable with durable checkpoints")
	}

	progress, _ := repo.ListRecipientProgress(context.Background(), "tenant-1", "job-1")
	var sentCount, pendingCount int
	for _, item := range progress {
		switch item.DeliveryStatus {
		case recipientStatusSent:
			sentCount++
		case recipientStatusPending:
			pendingCount++
		}
	}
	if sentCount != 1 || pendingCount != 1 {
		t.Fatalf("unexpected progress after partial retryable failure: %+v", progress)
	}
}

func TestDeliveryProcessorDoesNotTreatEmptySendResultAsSuccess(t *testing.T) {
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
	}}
	sender := &senderMock{results: []*instance.SendTextResult{{}}}
	processor := newDeliveryProcessor(newBroadcastRepoMock(), instanceRepoMock{}, contacts, sender, nilLogger())

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

func TestDeliveryProcessorResumeSkipsAlreadySentRecipients(t *testing.T) {
	repo := newBroadcastRepoMock()
	_ = repo.SeedRecipientProgress(context.Background(), []repository.BroadcastRecipientProgress{
		{BroadcastID: "job-1", TenantID: "tenant-1", InstanceID: "instance-1", Phone: "521111111111", DeliveryStatus: recipientStatusRead, AttemptCount: 1},
		{BroadcastID: "job-1", TenantID: "tenant-1", InstanceID: "instance-1", Phone: "522222222222", DeliveryStatus: recipientStatusPending},
	})
	sender := &senderMock{}
	processor := newDeliveryProcessor(repo, instanceRepoMock{}, contactRepoMock{}, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(sender.calls) != 1 || sender.calls[0].Number != "522222222222" {
		t.Fatalf("expected resume to skip terminal recipient states, got %+v", sender.calls)
	}
}

func TestDeliveryProcessorRetryableFailureLeavesRecipientPendingForResume(t *testing.T) {
	repo := newBroadcastRepoMock()
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c2", TenantID: "tenant-1", Phone: "522222222222", InstanceID: "instance-1"},
	}}
	sender := &senderMock{errs: []error{nil, errors.New("runtime unavailable")}}
	processor := newDeliveryProcessor(repo, instanceRepoMock{}, contacts, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err == nil || !isRetryableProcessorError(err) {
		t.Fatalf("expected retryable error, got %v", err)
	}

	progress, _ := repo.ListRecipientProgress(context.Background(), "tenant-1", "job-1")
	if len(progress) != 2 {
		t.Fatalf("expected 2 recipient progress records, got %d", len(progress))
	}
	var sentCount, pendingAttempts int
	for _, item := range progress {
		switch item.Phone {
		case "521111111111":
			if item.DeliveryStatus == recipientStatusSent {
				sentCount++
			}
		case "522222222222":
			if item.DeliveryStatus == recipientStatusPending && item.AttemptCount == 1 {
				pendingAttempts++
			}
		}
	}
	if sentCount != 1 || pendingAttempts != 1 {
		t.Fatalf("unexpected persisted progress: %+v", progress)
	}
}

func TestDeliveryProcessorPermanentRecipientFailureDoesNotBlockOtherRecipients(t *testing.T) {
	repo := newBroadcastRepoMock()
	contacts := contactRepoMock{contacts: []repository.Contact{
		{ID: "c1", TenantID: "tenant-1", Phone: "521111111111", InstanceID: "instance-1"},
		{ID: "c2", TenantID: "tenant-1", Phone: "522222222222", InstanceID: "instance-1"},
	}}
	sender := &senderMock{errs: []error{errors.New("validation failed: bad number"), nil}}
	processor := newDeliveryProcessor(repo, instanceRepoMock{}, contacts, sender, nilLogger())

	err := processor.Process(context.Background(), repository.BroadcastJob{
		ID:         "job-1",
		TenantID:   "tenant-1",
		InstanceID: "instance-1",
		Message:    "hello",
	})
	if err != nil {
		t.Fatalf("expected processor to continue after permanent recipient failure, got %v", err)
	}

	progress, _ := repo.ListRecipientProgress(context.Background(), "tenant-1", "job-1")
	var failedCount, sentCount int
	for _, item := range progress {
		switch item.DeliveryStatus {
		case recipientStatusFailed:
			failedCount++
		case recipientStatusSent:
			sentCount++
		}
	}
	if failedCount != 1 || sentCount != 1 {
		t.Fatalf("unexpected persisted progress: %+v", progress)
	}
}
