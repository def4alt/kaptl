package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/models"
	"github.com/def4alt/kaptl/internal/reporting"
	"github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

func TestValuationJobLifecycleUsesNativeTransactionSnapshot(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	binDir := filepath.Join(t.TempDir(), "bin")
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(9881).Database("kaptl").Username("postgres").Password("postgres").DataPath(dataDir).BinariesPath(binDir))
	if err := pg.Start(); err != nil {
		t.Fatal(err)
	}
	defer pg.Stop()
	pool, err := pgxpool.New(ctx, "postgres://postgres:postgres@localhost:9881/kaptl?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, file := range []string{"001_init.sql", "001_init.sql", "002_multi_currency.sql", "002_multi_currency.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (telegram_id) VALUES (1);
		INSERT INTO accounts (id, user_id, name, emoji, currency) VALUES (1, 1, 'Mono', '💳', 'UAH');
		INSERT INTO categories (id, user_id, name, emoji) VALUES (1, 1, 'Food', '🍞');
	`)
	if err != nil {
		t.Fatal(err)
	}
	var historicalID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO transactions(user_id, account_id, category_id, type, amount, created_at)
		VALUES (1, 1, 1, 'expense', 617, '2026-07-20T12:00:00Z') RETURNING id
	`).Scan(&historicalID); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"003_reporting_valuations.sql", "003_reporting_valuations.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	var halfEvenDown, halfEvenUp decimal.Decimal
	if err := pool.QueryRow(ctx, `SELECT round_half_even_2(15.045), round_half_even_2(15.055)`).Scan(&halfEvenDown, &halfEvenUp); err != nil || halfEvenDown.StringFixed(2) != "15.04" || halfEvenUp.StringFixed(2) != "15.06" {
		t.Fatalf("PostgreSQL half-even down=%s up=%s err=%v", halfEvenDown, halfEvenUp, err)
	}
	var historicalJobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM valuation_jobs WHERE transaction_id=$1`, historicalID).Scan(&historicalJobs); err != nil || historicalJobs != 1 {
		t.Fatalf("historical backfill jobs=%d err=%v", historicalJobs, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM transactions WHERE id=$1`, historicalID); err != nil {
		t.Fatal(err)
	}
	database := &DB{Pool: pool}
	if _, err := database.SetBudget(ctx, 1, 1, 0, 1, decimal.NewFromInt(100)); err != nil {
		t.Fatal(err)
	}
	tx, err := database.CreateTransaction(ctx, 1, 1, int64ptr(1), "expense", decimal.NewFromInt(721), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetReportingSummary(ctx, 1); err == nil {
		t.Fatal("reporting summary succeeded with a missing valuation")
	} else {
		var pending *models.ValuationsPendingError
		if !errors.As(err, &pending) || pending.Count != 1 {
			t.Fatalf("pending error = %v", err)
		}
	}

	job, err := database.ClaimValuationJob(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job == nil || job.TransactionID != tx.ID || job.Native.Currency != "UAH" || job.Native.Amount.StringFixed(2) != "721.00" {
		t.Fatalf("job %#v", job)
	}
	completion := reporting.Completion{Job: *job, Amount: decimal.RequireFromString("15.04"), Quote: fx.Quote{Source: "UAH", Target: "EUR", Rate: decimal.RequireFromString("0.02085992"), EffectiveAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC), ObservedAt: time.Now(), Provider: "NBU"}}
	if err := database.CompleteValuation(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if cached, err := database.FindFXQuote(ctx, "NBU", "UAH", "EUR", time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)); err != nil || cached == nil {
		t.Fatalf("exact-date quote cache=%#v err=%v", cached, err)
	}
	if cached, err := database.FindFXQuote(ctx, "NBU", "UAH", "EUR", time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)); err != nil || cached != nil {
		t.Fatalf("non-exact quote cache=%#v err=%v", cached, err)
	}
	var usdQuoteID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO fx_quotes(source_currency, target_currency, rate, effective_at, observed_at, provider)
		VALUES ('UAH', 'USD', 0.1, DATE '2026-07-24', clock_timestamp(), 'test') RETURNING id
	`).Scan(&usdQuoteID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO transaction_valuations(transaction_id, target_currency, purpose, policy_version, amount, fx_quote_id)
		VALUES ($1, 'USD', 'budget', 1, 72.10, $2)
	`, tx.ID, usdQuoteID)
	if err == nil || !strings.Contains(err.Error(), "transaction_valuations_policy_shape_check") {
		t.Fatalf("non-EUR v1 valuation error = %v", err)
	}
	var amount decimal.Decimal
	if err := pool.QueryRow(ctx, `SELECT amount FROM transaction_valuations WHERE transaction_id=$1`, tx.ID).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount.StringFixed(2) != "15.04" {
		t.Fatalf("valuation amount %s", amount)
	}

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `SELECT pg_advisory_xact_lock(1)`); err != nil {
		t.Fatal(err)
	}
	var concurrentID int64
	if err := writer.QueryRow(ctx, `
		INSERT INTO transactions(user_id, account_id, category_id, type, amount)
		VALUES (1, 1, 1, 'expense', 2) RETURNING id
	`).Scan(&concurrentID); err != nil {
		t.Fatal(err)
	}
	summaryResult := make(chan error, 1)
	go func() {
		_, summaryErr := database.GetReportingSummary(ctx, 1)
		summaryResult <- summaryErr
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted)`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("reporting summary did not wait for writer advisory lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var waitedPending *models.ValuationsPendingError
	if err := <-summaryResult; !errors.As(err, &waitedPending) || waitedPending.Count != 1 {
		t.Fatalf("summary after waiting for writer = %#v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM transactions WHERE id=$1`, concurrentID); err != nil {
		t.Fatal(err)
	}

	summary, err := database.GetReportingSummary(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Currency != "EUR" || len(summary.Rows) != 1 || summary.Rows[0].Spent.StringFixed(2) != "15.04" {
		t.Fatalf("reporting summary = %#v", summary)
	}
	if len(summary.ReadyToAssign) != 1 || summary.ReadyToAssign[0].Amount.StringFixed(2) != "-100.00" {
		t.Fatalf("Ready to Assign = %#v", summary.ReadyToAssign)
	}
	if _, err := pool.Exec(ctx, `UPDATE fx_quotes SET rate=1 WHERE provider='NBU'`); err == nil || !strings.Contains(err.Error(), "FX quotes are immutable") {
		t.Fatalf("quote update error = %v", err)
	}

	invalid, err := database.CreateTransaction(ctx, 1, 1, int64ptr(1), "expense", decimal.NewFromInt(5), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var identityQuoteID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO fx_quotes(source_currency, target_currency, rate, effective_at, observed_at, provider)
		VALUES ('EUR', 'EUR', 1, '2026-07-26', clock_timestamp(), 'IDENTITY') RETURNING id
	`).Scan(&identityQuoteID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO transaction_valuations(transaction_id, target_currency, purpose, policy_version, amount, fx_quote_id)
		VALUES ($1, 'EUR', 'budget', 1, 5, $2)
	`, invalid.ID, identityQuoteID)
	if err == nil || !strings.Contains(err.Error(), "quote currencies do not match transaction") {
		t.Fatalf("invalid quote linkage error = %v", err)
	}
	var nbuQuoteID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM fx_quotes WHERE provider='NBU'`).Scan(&nbuQuoteID); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO transaction_valuations(transaction_id, target_currency, purpose, policy_version, amount, fx_quote_id)
		VALUES ($1, 'EUR', 'budget', 1, 5, $2)
	`, invalid.ID, nbuQuoteID)
	if err == nil || !strings.Contains(err.Error(), "valuation amount does not match") {
		t.Fatalf("forged valuation amount error = %v", err)
	}
	invalidJob, err := database.ClaimValuationJob(ctx, "validation-worker", time.Minute)
	if err != nil || invalidJob == nil || invalidJob.TransactionID != invalid.ID {
		t.Fatalf("validation job=%#v err=%v", invalidJob, err)
	}
	wrongAmount := reporting.Completion{Job: *invalidJob, Amount: decimal.NewFromInt(5), Quote: completion.Quote}
	if err := database.CompleteValuation(ctx, wrongAmount); err == nil || !strings.Contains(err.Error(), "completion amount does not match") {
		t.Fatalf("invalid completion amount error = %v", err)
	}
	if err := database.FailValuation(ctx, *invalidJob, errors.New("temporary completion failure")); err != nil {
		t.Fatal(err)
	}
	var retryStatus string
	var retryAttempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM valuation_jobs WHERE transaction_id=$1`, invalid.ID).Scan(&retryStatus, &retryAttempts); err != nil || retryStatus != "retry" || retryAttempts != 1 {
		t.Fatalf("retry status=%q attempts=%d err=%v", retryStatus, retryAttempts, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM transactions WHERE id=$1`, invalid.ID); err != nil {
		t.Fatal(err)
	}
	tiny, err := database.CreateTransaction(ctx, 1, 1, int64ptr(1), "expense", decimal.RequireFromString("0.01"), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	tinyJob, err := database.ClaimValuationJob(ctx, "tiny-worker", time.Minute)
	if err != nil || tinyJob == nil || tinyJob.TransactionID != tiny.ID {
		t.Fatalf("tiny job=%#v err=%v", tinyJob, err)
	}
	if err := database.CompleteValuation(ctx, reporting.Completion{Job: *tinyJob, Amount: decimal.Zero, Quote: completion.Quote}); err != nil {
		t.Fatalf("zero-rounded valuation: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM transactions WHERE id=$1`, tiny.ID); err != nil {
		t.Fatal(err)
	}

	second, err := database.CreateTransaction(ctx, 1, 1, int64ptr(1), "expense", decimal.NewFromInt(10), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE valuation_jobs SET target_currency='USD' WHERE transaction_id=$1`, second.ID); err == nil || !strings.Contains(err.Error(), "valuation_jobs_policy_shape_check") {
		t.Fatalf("non-EUR v1 job error = %v", err)
	}
	conflictingJob, err := database.ClaimValuationJob(ctx, "worker-2", time.Minute)
	if err != nil || conflictingJob == nil || conflictingJob.TransactionID != second.ID {
		t.Fatalf("conflicting job=%#v err=%v", conflictingJob, err)
	}
	staleJob := *conflictingJob
	if _, err := pool.Exec(ctx, `UPDATE valuation_jobs SET locked_until=clock_timestamp()-interval '1 second' WHERE transaction_id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.FailValuation(ctx, staleJob, errors.New("late provider failure")); err == nil || !strings.Contains(err.Error(), "lease is no longer owned") {
		t.Fatalf("expired lease failure error = %v", err)
	}
	conflictingJob, err = database.ClaimValuationJob(ctx, "worker-2", time.Minute)
	if err != nil || conflictingJob == nil || conflictingJob.LeaseToken == staleJob.LeaseToken {
		t.Fatalf("reclaimed job=%#v stale=%#v err=%v", conflictingJob, staleJob, err)
	}
	staleCompletion := reporting.Completion{Job: staleJob, Amount: decimal.RequireFromString("0.21"), Quote: completion.Quote}
	if err := database.CompleteValuation(ctx, staleCompletion); err == nil || !strings.Contains(err.Error(), "lease is no longer owned") {
		t.Fatalf("stale lease completion error = %v", err)
	}
	conflicting := reporting.Completion{Job: *conflictingJob, Amount: decimal.RequireFromString("0.21"), Quote: fx.Quote{Source: "UAH", Target: "EUR", Rate: decimal.RequireFromString("0.021"), EffectiveAt: completion.Quote.EffectiveAt, ObservedAt: time.Now(), Provider: "NBU"}}
	if err := database.CompleteValuation(ctx, conflicting); err == nil || !strings.Contains(err.Error(), "conflicts with immutable cached rate") {
		t.Fatalf("quote conflict error = %v", err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM valuation_jobs WHERE transaction_id=$1`, second.ID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("conflicting job count=%d err=%v", jobs, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE valuation_jobs SET status='retry', attempts=12, next_attempt_at=clock_timestamp(),
			locked_by=NULL, locked_until=NULL, locked_token=NULL
		WHERE transaction_id=$1
	`, second.ID); err != nil {
		t.Fatal(err)
	}
	if exhausted, err := database.ClaimValuationJob(ctx, "worker-3", time.Minute); err != nil || exhausted != nil {
		t.Fatalf("exhausted claim=%#v err=%v", exhausted, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM valuation_jobs WHERE transaction_id=$1`, second.ID).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("job status=%q err=%v", status, err)
	}
	_, err = database.GetReportingSummary(ctx, 1)
	var failed *models.ValuationsPendingError
	if !errors.As(err, &failed) || failed.Failed != 1 {
		t.Fatalf("failed valuation summary error = %#v", err)
	}
}

func int64ptr(v int64) *int64 { return &v }
