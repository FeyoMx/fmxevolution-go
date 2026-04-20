package dashboard

import (
	"context"
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/repository"
)

type metricsInstanceRepoMock struct {
	items []repository.Instance
}

func (m metricsInstanceRepoMock) ListByTenant(_ context.Context, tenantID string) ([]repository.Instance, error) {
	items := make([]repository.Instance, 0, len(m.items))
	for _, item := range m.items {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	return items, nil
}

type metricsContactRepoMock struct {
	totals map[string]int64
}

func (m metricsContactRepoMock) CountContactsByTenant(_ context.Context, tenantID string) (int64, error) {
	return m.totals[tenantID], nil
}

type metricsCounterMock struct {
	totals             map[string]int64
	recipientSummaries map[string]repository.BroadcastRecipientAnalytics
}

func (m metricsCounterMock) CountByTenant(_ context.Context, tenantID string) (int64, error) {
	return m.totals[tenantID], nil
}

func (m metricsCounterMock) SummarizeRecipientProgressByTenant(_ context.Context, tenantID string) (repository.BroadcastRecipientAnalytics, error) {
	return m.recipientSummaries[tenantID], nil
}

type metricsRuntimeRepoMock struct {
	states []repository.RuntimeSessionState
}

func (m metricsRuntimeRepoMock) ListStatesByTenant(_ context.Context, tenantID string) ([]repository.RuntimeSessionState, error) {
	items := make([]repository.RuntimeSessionState, 0, len(m.states))
	for _, item := range m.states {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	return items, nil
}

func TestMetricsUsesRealStoredCountsAndRuntimeHealth(t *testing.T) {
	service := NewService(
		metricsInstanceRepoMock{items: []repository.Instance{
			{ID: "i1", TenantID: "tenant-1", Status: "created"},
			{ID: "i2", TenantID: "tenant-1", Status: "created"},
			{ID: "i3", TenantID: "tenant-1", Status: "open"},
		}},
		metricsContactRepoMock{totals: map[string]int64{"tenant-1": 7}},
		metricsCounterMock{totals: map[string]int64{"tenant-1": 42}},
		metricsCounterMock{
			totals: map[string]int64{"tenant-1": 3},
			recipientSummaries: map[string]repository.BroadcastRecipientAnalytics{
				"tenant-1": {
					TrackedBroadcasts: 2,
					TotalRecipients:   10,
					Attempted:         8,
					Sent:              6,
					Delivered:         4,
					Read:              2,
					Failed:            1,
					Pending:           3,
				},
			},
		},
		metricsRuntimeRepoMock{states: []repository.RuntimeSessionState{
			{TenantID: "tenant-1", InstanceID: "i1", Connected: true, Status: "open"},
			{TenantID: "tenant-1", InstanceID: "i2", PairingActive: true, Status: "connecting"},
		}},
	)

	snapshot, err := service.Metrics(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}

	if snapshot.ContactsTotal != 7 {
		t.Fatalf("expected contacts total 7, got %d", snapshot.ContactsTotal)
	}
	if snapshot.MessagesTotal != 42 {
		t.Fatalf("expected messages total 42, got %d", snapshot.MessagesTotal)
	}
	if !snapshot.MessagesTotalPartial {
		t.Fatal("expected messages total to be marked partial")
	}
	if snapshot.BroadcastTotal != 3 {
		t.Fatalf("expected broadcast total 3, got %d", snapshot.BroadcastTotal)
	}
	if snapshot.BroadcastRecipients.TotalRecipients != 10 || snapshot.BroadcastRecipients.Sent != 6 || snapshot.BroadcastRecipients.Delivered != 4 || snapshot.BroadcastRecipients.Read != 2 {
		t.Fatalf("unexpected broadcast recipient analytics: %+v", snapshot.BroadcastRecipients)
	}
	if !snapshot.BroadcastRecipients.Partial {
		t.Fatal("expected broadcast recipient analytics to be partial when not every broadcast is tracked")
	}
	if snapshot.InstancesActive != 2 {
		t.Fatalf("expected 2 active instances from runtime-backed counts plus fallback, got %d", snapshot.InstancesActive)
	}
	if snapshot.RuntimeHealthy != 1 || snapshot.RuntimeDegraded != 1 || snapshot.RuntimeUnknown != 1 {
		t.Fatalf("unexpected runtime health counts: %+v", snapshot)
	}
	if !snapshot.RuntimeHealthPartial {
		t.Fatal("expected runtime health to be partial when some instances have no state")
	}
}

func TestMetricsFallsBackToInstanceStatusWhenRuntimeStateIsMissing(t *testing.T) {
	service := NewService(
		metricsInstanceRepoMock{items: []repository.Instance{
			{ID: "i1", TenantID: "tenant-1", Status: "open"},
			{ID: "i2", TenantID: "tenant-1", Status: "connected"},
		}},
		metricsContactRepoMock{},
		metricsCounterMock{},
		metricsCounterMock{},
		metricsRuntimeRepoMock{},
	)

	snapshot, err := service.Metrics(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}

	if snapshot.InstancesActive != 2 {
		t.Fatalf("expected fallback active count of 2, got %d", snapshot.InstancesActive)
	}
	if snapshot.RuntimeUnknown != 2 {
		t.Fatalf("expected 2 runtime unknown instances, got %d", snapshot.RuntimeUnknown)
	}
}
