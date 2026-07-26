package reporting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/def4alt/kaptl/internal/reporting"
	"github.com/shopspring/decimal"
)

type fakeStore struct {
	job         *reporting.Job
	quote       *fx.Quote
	completed   *reporting.Completion
	completeErr error
	failed      error
}

func (s *fakeStore) FindFXQuote(context.Context, string, string, string, time.Time) (*fx.Quote, error) {
	return s.quote, nil
}

func (s *fakeStore) ClaimValuationJob(context.Context, string, time.Duration) (*reporting.Job, error) {
	job := s.job
	s.job = nil
	return job, nil
}
func (s *fakeStore) CompleteValuation(_ context.Context, completion reporting.Completion) error {
	s.completed = &completion
	return s.completeErr
}
func (s *fakeStore) FailValuation(_ context.Context, _ reporting.Job, cause error) error {
	s.failed = cause
	return nil
}

type fakeProvider struct {
	quote fx.Quote
	err   error
	calls int
}

func (p *fakeProvider) Name() string { return "NBU" }

func (p *fakeProvider) Quote(context.Context, string, string, time.Time) (fx.Quote, error) {
	p.calls++
	return p.quote, p.err
}

func TestWorkerStoresHistoricalValuationFromClaimedJob(t *testing.T) {
	native, _ := money.Parse("721", "UAH")
	store := &fakeStore{job: &reporting.Job{TransactionID: 5, Native: native, TargetCurrency: "EUR", Purpose: "budget", PolicyVersion: 1, OccurredAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)}}
	provider := &fakeProvider{quote: fx.Quote{Source: "UAH", Target: "EUR", Rate: decimal.RequireFromString("0.02085992"), Provider: "NBU", EffectiveAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)}}
	worker := reporting.NewWorker(store, provider, "test-worker")

	didWork, err := worker.ProcessOne(context.Background())
	if err != nil || !didWork {
		t.Fatalf("didWork=%v err=%v", didWork, err)
	}
	if store.completed == nil || store.completed.Amount.StringFixed(2) != "15.04" {
		t.Fatalf("completion %#v", store.completed)
	}
	if store.completed.Quote.Provider != "NBU" {
		t.Fatalf("quote not preserved: %#v", store.completed.Quote)
	}
}

func TestWorkerRecordsProviderFailureForRetry(t *testing.T) {
	native, _ := money.Parse("721", "UAH")
	store := &fakeStore{job: &reporting.Job{TransactionID: 5, Native: native, TargetCurrency: "EUR"}}
	provider := &fakeProvider{err: errors.New("NBU unavailable")}

	didWork, err := reporting.NewWorker(store, provider, "test-worker").ProcessOne(context.Background())
	if err != nil || !didWork {
		t.Fatalf("didWork=%v err=%v", didWork, err)
	}
	if store.failed == nil || store.completed != nil {
		t.Fatalf("failed=%v completed=%#v", store.failed, store.completed)
	}
}

func TestWorkerRecordsCompletionFailureForRetry(t *testing.T) {
	native, _ := money.Parse("19", "EUR")
	store := &fakeStore{
		job:         &reporting.Job{TransactionID: 2, Native: native, TargetCurrency: "EUR", OccurredAt: time.Now()},
		completeErr: errors.New("immutable quote conflict"),
	}

	didWork, err := reporting.NewWorker(store, &fakeProvider{}, "test-worker").ProcessOne(context.Background())
	if err != nil || !didWork {
		t.Fatalf("didWork=%v err=%v", didWork, err)
	}
	if store.failed == nil || store.failed.Error() != "immutable quote conflict" {
		t.Fatalf("failed=%v", store.failed)
	}
}

func TestWorkerUsesIdentityQuoteWithoutCallingProvider(t *testing.T) {
	native, _ := money.Parse("19", "EUR")
	occurredAt := time.Date(2026, 7, 23, 22, 0, 0, 0, time.UTC) // 2026-07-24 in Kyiv
	store := &fakeStore{job: &reporting.Job{TransactionID: 2, Native: native, TargetCurrency: "EUR", OccurredAt: occurredAt}}
	provider := &fakeProvider{err: errors.New("must not be called")}

	_, err := reporting.NewWorker(store, provider, "test-worker").ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || store.completed == nil || store.completed.Quote.Provider != "IDENTITY" {
		t.Fatalf("calls=%d completion=%#v", provider.calls, store.completed)
	}
	if got := store.completed.Quote.EffectiveAt.UTC().Format("2006-01-02"); got != "2026-07-24" {
		t.Fatalf("identity effective date = %s, want 2026-07-24", got)
	}
}

func TestWorkerUsesCachedHistoricalQuoteWithoutCallingProvider(t *testing.T) {
	native, _ := money.Parse("721", "UAH")
	quote := fx.Quote{Source: "UAH", Target: "EUR", Rate: decimal.RequireFromString("0.02085992"), Provider: "NBU", EffectiveAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)}
	store := &fakeStore{job: &reporting.Job{TransactionID: 5, Native: native, TargetCurrency: "EUR", OccurredAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)}, quote: &quote}
	provider := &fakeProvider{err: errors.New("must not be called")}

	_, err := reporting.NewWorker(store, provider, "test-worker").ProcessOne(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 0 || store.completed == nil || !store.completed.Quote.EffectiveAt.Equal(quote.EffectiveAt) {
		t.Fatalf("calls=%d completion=%#v", provider.calls, store.completed)
	}
}
