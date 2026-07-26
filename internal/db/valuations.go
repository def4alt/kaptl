package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/def4alt/kaptl/internal/reporting"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const maxValuationAttempts = 12

func (d *DB) FindFXQuote(ctx context.Context, provider, source, target string, at time.Time) (*fx.Quote, error) {
	var quote fx.Quote
	err := d.Pool.QueryRow(ctx, `
		SELECT id, source_currency, target_currency, rate, effective_at, observed_at, provider
		FROM fx_quotes
		WHERE provider=$1 AND source_currency=$2 AND target_currency=$3
		  AND effective_at = ($4::timestamptz AT TIME ZONE 'Europe/Kyiv')::date
		LIMIT 1
	`, provider, source, target, at).Scan(
		&quote.ID, &quote.Source, &quote.Target, &quote.Rate,
		&quote.EffectiveAt, &quote.ObservedAt, &quote.Provider,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find FX quote: %w", err)
	}
	return &quote, nil
}

func (d *DB) ClaimValuationJob(ctx context.Context, workerID string, lease time.Duration) (*reporting.Job, error) {
	leaseBytes := make([]byte, 16)
	if _, err := rand.Read(leaseBytes); err != nil {
		return nil, fmt.Errorf("generate valuation lease token: %w", err)
	}
	leaseToken := hex.EncodeToString(leaseBytes)
	var job reporting.Job
	var amount decimal.Decimal
	err := d.Pool.QueryRow(ctx, `
		WITH exhausted AS (
			UPDATE valuation_jobs
			SET status='failed', failed_at=clock_timestamp(),
				last_error=COALESCE(last_error, 'maximum attempts exhausted after abandoned lease'),
				locked_by=NULL, locked_until=NULL, locked_token=NULL
			WHERE status IN ('pending', 'retry') AND attempts >= $4
			  AND (locked_until IS NULL OR locked_until < clock_timestamp())
		), candidate AS (
			SELECT j.transaction_id, j.target_currency, j.purpose, j.policy_version,
				j.attempts, t.amount, t.currency, t.created_at
			FROM valuation_jobs j
			JOIN transactions t ON t.id = j.transaction_id
			WHERE j.next_attempt_at <= clock_timestamp()
				AND j.status IN ('pending', 'retry')
				AND j.attempts < $4
				AND (j.locked_until IS NULL OR j.locked_until < clock_timestamp())
			ORDER BY j.next_attempt_at, j.transaction_id
			FOR UPDATE OF j SKIP LOCKED
			LIMIT 1
		), claimed AS (
			UPDATE valuation_jobs j
			SET locked_by = $1, locked_until = clock_timestamp() + $2::interval,
				locked_token = $3, attempts = j.attempts + 1
			FROM candidate c
			WHERE j.transaction_id = c.transaction_id
				AND j.target_currency = c.target_currency
				AND j.purpose = c.purpose
				AND j.policy_version = c.policy_version
			RETURNING c.transaction_id, c.target_currency, c.purpose, c.policy_version,
				j.attempts, c.amount, c.currency, c.created_at
		)
		SELECT transaction_id, target_currency, purpose, policy_version,
			attempts, amount, currency, created_at
		FROM claimed
	`, workerID, lease.String(), leaseToken, maxValuationAttempts).Scan(
		&job.TransactionID, &job.TargetCurrency, &job.Purpose, &job.PolicyVersion,
		&job.Attempts, &amount, &job.Native.Currency, &job.OccurredAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim valuation job: %w", err)
	}
	job.Native = money.Money{Amount: amount, Currency: job.Native.Currency}
	job.LockedBy = workerID
	job.LeaseToken = leaseToken
	return &job, nil
}

func (d *DB) CompleteValuation(ctx context.Context, completion reporting.Completion) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin valuation completion: %w", err)
	}
	defer tx.Rollback(ctx)
	job := completion.Job
	var userID int64
	var nativeAmount decimal.Decimal
	var nativeCurrency string
	if err := tx.QueryRow(ctx, `
		SELECT user_id, amount, currency FROM transactions WHERE id=$1
	`, job.TransactionID).Scan(&userID, &nativeAmount, &nativeCurrency); err != nil {
		return fmt.Errorf("get valuation owner and native amount: %w", err)
	}
	// All user-scoped writers acquire the advisory lock before row locks. This
	// avoids deadlock with destructive operations that cascade into job rows.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return fmt.Errorf("lock valuation owner: %w", err)
	}
	var lockedTransactionID int64
	if err := tx.QueryRow(ctx, `
		SELECT transaction_id
		FROM valuation_jobs
		WHERE transaction_id=$1 AND target_currency=$2 AND purpose=$3
			AND policy_version=$4 AND locked_by=$5 AND locked_token=$6
			AND locked_until >= clock_timestamp()
		FOR UPDATE
	`, job.TransactionID, job.TargetCurrency, job.Purpose, job.PolicyVersion, job.LockedBy, job.LeaseToken).Scan(&lockedTransactionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("valuation job lease is no longer owned by %s", job.LockedBy)
		}
		return fmt.Errorf("lock valuation job: %w", err)
	}
	if completion.Quote.Source != nativeCurrency || completion.Quote.Target != job.TargetCurrency {
		return fmt.Errorf("completion quote currencies do not match native transaction and job")
	}
	expected, err := fx.Convert(money.Money{Amount: nativeAmount, Currency: nativeCurrency}, completion.Quote)
	if err != nil {
		return fmt.Errorf("validate completion conversion: %w", err)
	}
	if !expected.Amount.Equal(completion.Amount) {
		return fmt.Errorf("completion amount does not match quote: got %s, want %s", completion.Amount, expected.Amount)
	}

	var quoteID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO fx_quotes (
			source_currency, target_currency, rate, effective_at, observed_at, provider
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (provider, source_currency, target_currency, effective_at)
		DO UPDATE SET provider = fx_quotes.provider
		WHERE fx_quotes.rate = EXCLUDED.rate
		RETURNING id
	`, completion.Quote.Source, completion.Quote.Target, completion.Quote.Rate,
		completion.Quote.EffectiveAt, completion.Quote.ObservedAt, completion.Quote.Provider).Scan(&quoteID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("FX quote conflicts with immutable cached rate")
		}
		return fmt.Errorf("store FX quote: %w", err)
	}
	var valuationTransactionID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO transaction_valuations (
			transaction_id, target_currency, purpose, policy_version, amount, fx_quote_id
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (transaction_id, target_currency, purpose, policy_version)
		DO UPDATE SET amount = transaction_valuations.amount
		WHERE transaction_valuations.amount = EXCLUDED.amount
		  AND transaction_valuations.fx_quote_id = EXCLUDED.fx_quote_id
		RETURNING transaction_id
	`, job.TransactionID, job.TargetCurrency, job.Purpose, job.PolicyVersion, completion.Amount, quoteID).Scan(&valuationTransactionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("transaction valuation conflicts with immutable snapshot")
		}
		return fmt.Errorf("store transaction valuation: %w", err)
	}
	command, err := tx.Exec(ctx, `
		UPDATE valuation_jobs
		SET status='completed', locked_by=NULL, locked_until=NULL, locked_token=NULL,
			last_error=NULL, completed_at=clock_timestamp(), failed_at=NULL
		WHERE transaction_id=$1 AND target_currency=$2 AND purpose=$3
			AND policy_version=$4 AND locked_by=$5 AND locked_token=$6
	`, job.TransactionID, job.TargetCurrency, job.Purpose, job.PolicyVersion, job.LockedBy, job.LeaseToken)
	if err != nil {
		return fmt.Errorf("complete valuation job: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("valuation job lease is no longer owned by %s", job.LockedBy)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit valuation: %w", err)
	}
	return nil
}

func (d *DB) FailValuation(ctx context.Context, job reporting.Job, cause error) error {
	attempts := job.Attempts
	if attempts < 1 {
		attempts = 1
	}
	status := "retry"
	if attempts >= maxValuationAttempts {
		status = "failed"
	}
	delay := time.Minute * time.Duration(1<<min(attempts, 6))
	command, err := d.Pool.Exec(ctx, `
		UPDATE valuation_jobs
		SET status=$6, attempts=$7, last_error=left($1, 1000), next_attempt_at=clock_timestamp()+$2::interval,
			locked_by=NULL, locked_until=NULL, locked_token=NULL,
			failed_at=CASE WHEN $6='failed' THEN clock_timestamp() ELSE NULL END
		WHERE transaction_id=$3 AND target_currency=$4 AND purpose=$5 AND policy_version=$8
			AND locked_by=$9 AND locked_token=$10
			AND locked_until >= clock_timestamp()
	`, cause.Error(), delay.String(), job.TransactionID, job.TargetCurrency, job.Purpose, status, attempts, job.PolicyVersion, job.LockedBy, job.LeaseToken)
	if err != nil {
		return fmt.Errorf("mark valuation job %s: %w", status, err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("valuation job lease is no longer owned by %s", job.LockedBy)
	}
	return nil
}
