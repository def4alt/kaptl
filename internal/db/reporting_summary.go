package db

import (
	"context"
	"fmt"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

// GetReportingSummary values all spending and income in the user's immutable
// reporting currency. Rollover, summary rows, and Ready to Assign are produced
// from one lock-consistent state under the same per-user advisory lock.
func (d *DB) GetReportingSummary(ctx context.Context, userID int64) (*models.ReportingSummary, error) {
	// Read committed is deliberate: PostgreSQL establishes a repeatable-read
	// snapshot before a blocking advisory-lock call completes. Every writer of
	// financial facts takes this same user lock, so after acquisition they cannot
	// change between the statements below; read committed also sees the commit of
	// the writer we may have waited for.
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin reporting summary: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return nil, fmt.Errorf("lock reporting summary: %w", err)
	}
	var now time.Time
	var currency string
	if err := tx.QueryRow(ctx, `
		SELECT clock_timestamp(), reporting_currency
		FROM users WHERE telegram_id=$1
	`, userID).Scan(&now, &currency); err != nil {
		return nil, fmt.Errorf("load reporting settings: %w", err)
	}

	var pending, failed int
	if err := tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE j.status='failed')
		FROM transactions t
		LEFT JOIN transaction_valuations v
		  ON v.transaction_id=t.id
		 AND v.target_currency=$2
		 AND v.purpose='budget'
		 AND v.policy_version=1
		LEFT JOIN valuation_jobs j
		  ON j.transaction_id=t.id
		 AND j.target_currency=$2
		 AND j.purpose='budget'
		 AND j.policy_version=1
		WHERE t.user_id=$1
		  AND t.type IN ('expense','income')
		  AND v.transaction_id IS NULL
	`, userID, currency).Scan(&pending, &failed); err != nil {
		return nil, fmt.Errorf("check reporting valuation coverage: %w", err)
	}
	if pending > 0 {
		return nil, &models.ValuationsPendingError{Count: pending, Failed: failed}
	}

	expiredRows, err := tx.Query(ctx, `
		SELECT id, user_id, category_id, currency, period_start,
			interval_days, interval_months, amount, rollover, created_at
		FROM budgets
		WHERE user_id=$1 AND currency=$3
		  AND $2::timestamptz >= period_start + make_interval(days => interval_days, months => interval_months)
		FOR UPDATE
	`, userID, now, currency)
	if err != nil {
		return nil, fmt.Errorf("lock expired reporting budgets: %w", err)
	}
	expired, err := pgx.CollectRows(expiredRows, pgx.RowToStructByName[models.Budget])
	if err != nil {
		return nil, fmt.Errorf("collect expired reporting budgets: %w", err)
	}

	for _, budget := range expired {
		periodStart := budget.PeriodStart
		rollover := budget.Rollover
		for periods := 0; ; periods++ {
			if periods >= maxBudgetRolloverPeriods {
				return nil, fmt.Errorf("budget %d is more than %d periods behind", budget.ID, maxBudgetRolloverPeriods)
			}
			var periodEnd time.Time
			if err := tx.QueryRow(ctx, `
				SELECT $1::timestamptz + make_interval(days=>$2, months=>$3)
			`, periodStart, budget.IntervalDays, budget.IntervalMonths).Scan(&periodEnd); err != nil {
				return nil, fmt.Errorf("calculate reporting budget period end: %w", err)
			}
			if now.Before(periodEnd) {
				break
			}
			var spent decimal.Decimal
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(SUM(v.amount), 0)
				FROM transactions t
				JOIN transaction_valuations v
				  ON v.transaction_id=t.id
				 AND v.target_currency=$3
				 AND v.purpose='budget'
				 AND v.policy_version=1
				WHERE t.user_id=$1 AND t.category_id=$2 AND t.type='expense'
				  AND t.created_at >= $4 AND t.created_at < $5
			`, userID, budget.CategoryID, currency, periodStart, periodEnd).Scan(&spent); err != nil {
				return nil, fmt.Errorf("calculate reporting budget rollover: %w", err)
			}
			rollover = budget.Amount.Add(rollover).Sub(spent)
			if rollover.IsNegative() {
				rollover = decimal.Zero
			}
			periodStart = periodEnd
		}
		if _, err := tx.Exec(ctx, `
			UPDATE budgets SET rollover=$1, period_start=$2
			WHERE id=$3 AND user_id=$4
		`, rollover, periodStart, budget.ID, userID); err != nil {
			return nil, fmt.Errorf("update reporting budget rollover: %w", err)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT c.id, c.user_id, c.name, c.emoji, $3::text AS currency, c.created_at,
			COALESCE(SUM(v.amount), 0) AS spent,
			COALESCE(b.amount, 0) AS budget,
			COALESCE(b.rollover, 0) AS rollover,
			COALESCE(b.amount, 0) + COALESCE(b.rollover, 0) AS available,
			COALESCE(b.amount, 0) + COALESCE(b.rollover, 0) - COALESCE(SUM(v.amount), 0) AS remaining,
			0::integer AS missing_valuations,
			COALESCE(g.name, '') AS group_name,
			c.group_id
		FROM categories c
		LEFT JOIN budgets b ON b.user_id=$1 AND b.category_id=c.id AND b.currency=$3
		LEFT JOIN transactions t ON t.user_id=$1 AND t.category_id=c.id AND t.type='expense'
		  AND t.created_at >= COALESCE(b.period_start, date_trunc('month', $2::timestamptz))
		LEFT JOIN transaction_valuations v ON v.transaction_id=t.id
		  AND v.target_currency=$3 AND v.purpose='budget' AND v.policy_version=1
		LEFT JOIN category_groups g ON g.id=c.group_id AND g.user_id=c.user_id
		WHERE c.user_id=$1
		GROUP BY c.id, b.amount, b.rollover, g.name, c.group_id
		ORDER BY COALESCE(g.name, 'zzz'), c.name
	`, userID, now, currency)
	if err != nil {
		return nil, fmt.Errorf("query reporting summary: %w", err)
	}
	budgetRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.BudgetRow])
	if err != nil {
		return nil, fmt.Errorf("collect reporting summary: %w", err)
	}

	var ready decimal.Decimal
	if err := tx.QueryRow(ctx, `
		SELECT
		  COALESCE((
			SELECT SUM(v.amount)
			FROM transactions t
			JOIN transaction_valuations v ON v.transaction_id=t.id
			  AND v.target_currency=$2 AND v.purpose='budget' AND v.policy_version=1
			WHERE t.user_id=$1 AND t.type='income'
		  ), 0)
		  - COALESCE((SELECT SUM(amount) FROM budgets WHERE user_id=$1 AND currency=$2), 0)
	`, userID, currency).Scan(&ready); err != nil {
		return nil, fmt.Errorf("calculate reporting Ready to Assign: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reporting summary: %w", err)
	}
	return &models.ReportingSummary{
		Currency:      currency,
		Rows:          budgetRows,
		ReadyToAssign: []models.CurrencyAmount{{Currency: currency, Amount: ready}},
	}, nil
}
