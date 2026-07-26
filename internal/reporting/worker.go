package reporting

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

const leaseDuration = 2 * time.Minute

type Job struct {
	TransactionID  int64
	Native         money.Money
	TargetCurrency string
	Purpose        string
	PolicyVersion  int
	OccurredAt     time.Time
	Attempts       int
	LockedBy       string
	LeaseToken     string
}

type Completion struct {
	Job    Job
	Amount decimal.Decimal
	Quote  fx.Quote
}

type Store interface {
	FindFXQuote(ctx context.Context, provider, source, target string, at time.Time) (*fx.Quote, error)
	ClaimValuationJob(ctx context.Context, workerID string, lease time.Duration) (*Job, error)
	CompleteValuation(ctx context.Context, completion Completion) error
	FailValuation(ctx context.Context, job Job, cause error) error
}

type Worker struct {
	store    Store
	provider fx.RateProvider
	id       string
}

func NewWorker(store Store, provider fx.RateProvider, id string) *Worker {
	return &Worker{store: store, provider: provider, id: id}
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	job, err := w.store.ClaimValuationJob(ctx, w.id, leaseDuration)
	if err != nil || job == nil {
		return false, err
	}

	var quote fx.Quote
	if job.Native.Currency == job.TargetCurrency {
		effectiveAt, dateErr := reportingDate(job.OccurredAt)
		if dateErr != nil {
			if failErr := w.recordFailure(ctx, *job, dateErr); failErr != nil {
				return true, fmt.Errorf("record identity-date failure after %v: %w", dateErr, failErr)
			}
			return true, nil
		}
		quote = fx.Quote{
			Source: job.Native.Currency, Target: job.TargetCurrency,
			Rate: decimal.NewFromInt(1), EffectiveAt: effectiveAt,
			ObservedAt: time.Now(), Provider: "IDENTITY",
		}
	} else {
		cached, cacheErr := w.store.FindFXQuote(ctx, w.provider.Name(), job.Native.Currency, job.TargetCurrency, job.OccurredAt)
		if cacheErr != nil {
			if failErr := w.recordFailure(ctx, *job, cacheErr); failErr != nil {
				return true, fmt.Errorf("record quote-cache failure after %v: %w", cacheErr, failErr)
			}
			return true, nil
		}
		if cached != nil {
			quote = *cached
		} else {
			quote, err = w.provider.Quote(ctx, job.Native.Currency, job.TargetCurrency, job.OccurredAt)
		}
		if err != nil {
			if failErr := w.recordFailure(ctx, *job, err); failErr != nil {
				return true, fmt.Errorf("record valuation failure after %v: %w", err, failErr)
			}
			return true, nil
		}
	}

	converted, err := fx.Convert(job.Native, quote)
	if err != nil {
		if failErr := w.recordFailure(ctx, *job, err); failErr != nil {
			return true, fmt.Errorf("record conversion failure after %v: %w", err, failErr)
		}
		return true, nil
	}
	if err := w.store.CompleteValuation(ctx, Completion{Job: *job, Amount: converted.Amount, Quote: quote}); err != nil {
		if failErr := w.recordFailure(ctx, *job, err); failErr != nil {
			return true, fmt.Errorf("record completion failure after %v: %w", err, failErr)
		}
		return true, nil
	}
	return true, nil
}

func (w *Worker) recordFailure(ctx context.Context, job Job, cause error) error {
	if ctx.Err() == nil {
		return w.store.FailValuation(ctx, job, cause)
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.FailValuation(cleanupCtx, job, cause)
}

func reportingDate(at time.Time) (time.Time, error) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		return time.Time{}, err
	}
	year, month, day := at.In(kyiv).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		for {
			didWork, err := w.ProcessOne(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("valuation worker: %v", err)
				}
				break
			}
			if !didWork {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
