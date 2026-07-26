package db

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/jackc/pgx/v5"
)

const maxBudgetRolloverPeriods = 1000

// ─── Users ────────────────────────────────────────────────

func (d *DB) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*models.User, error) {
	u := &models.User{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO users (telegram_id, username, first_name, language_code)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (telegram_id) DO UPDATE
			SET username = EXCLUDED.username,
			    first_name = EXCLUDED.first_name
		RETURNING telegram_id, username, first_name, language_code, created_at
	`, telegramID, username, firstName, lang).Scan(
		&u.TelegramID, &u.Username, &u.FirstName, &u.LanguageCode, &u.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get or create user: %w", err)
	}
	return u, nil
}

// ─── Accounts ──────────────────────────────────────────────

func (d *DB) CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*models.Account, error) {
	currency, err := models.NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}
	a := &models.Account{}
	err = d.Pool.QueryRow(ctx, `
		INSERT INTO accounts (user_id, name, emoji, currency, initial_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, emoji, currency, initial_balance, created_at
	`, userID, name, emoji, currency, initialBalance).Scan(
		&a.ID, &a.UserID, &a.Name, &a.Emoji, &a.Currency, &a.InitialBalance, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return a, nil
}

func (d *DB) GetAccounts(ctx context.Context, userID int64) ([]models.Account, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT a.id, a.user_id, a.name, a.emoji, a.currency, a.initial_balance, a.created_at,
			COALESCE(a.initial_balance +
				SUM(CASE WHEN t.type = 'income' THEN t.amount ELSE 0 END) -
				SUM(CASE WHEN t.type IN ('expense','transfer') AND t.account_id = a.id THEN t.amount ELSE 0 END) +
				SUM(CASE WHEN t.transfer_account_id = a.id THEN t.amount ELSE 0 END), a.initial_balance) AS balance
		FROM accounts a
		LEFT JOIN transactions t ON t.user_id = $1
			AND (t.account_id = a.id OR t.transfer_account_id = a.id)
		WHERE a.user_id = $1
		GROUP BY a.id
		ORDER BY a.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get accounts: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Account])
}

func (d *DB) GetAccount(ctx context.Context, userID, id int64) (*models.Account, error) {
	a := &models.Account{}
	err := d.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, emoji, currency, initial_balance, created_at
		FROM accounts WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&a.ID, &a.UserID, &a.Name, &a.Emoji, &a.Currency, &a.InitialBalance, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get account %d: %w", id, err)
	}
	return a, nil
}

// ─── Category Groups ───────────────────────────────────────

func (d *DB) CreateGroup(ctx context.Context, userID int64, name, emoji string) (*models.CategoryGroup, error) {
	g := &models.CategoryGroup{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO category_groups (user_id, name, emoji)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, emoji, sort_order, created_at
	`, userID, name, emoji).Scan(&g.ID, &g.UserID, &g.Name, &g.Emoji, &g.SortOrder, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return g, nil
}

func (d *DB) GetGroups(ctx context.Context, userID int64) ([]models.CategoryGroup, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT id, user_id, name, emoji, sort_order, created_at
		FROM category_groups WHERE user_id = $1 ORDER BY sort_order, name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get groups: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.CategoryGroup])
}

func (d *DB) DeleteGroup(ctx context.Context, userID, groupID int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM category_groups WHERE id = $1 AND user_id = $2`, groupID, userID)
	return err
}

// ─── Categories ────────────────────────────────────────────

func (d *DB) CreateCategory(ctx context.Context, userID int64, name, emoji string, groupID *int64) (*models.Category, error) {
	c := &models.Category{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO categories (user_id, name, emoji, group_id)
		SELECT $1, $2, $3, $4
		WHERE $4::integer IS NULL OR EXISTS (
			SELECT 1 FROM category_groups g WHERE g.id = $4 AND g.user_id = $1
		)
		RETURNING id, user_id, group_id, name, emoji, created_at
	`, userID, name, emoji, groupID).Scan(
		&c.ID, &c.UserID, &c.GroupID, &c.Name, &c.Emoji, &c.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("category group does not belong to user")
	}
	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return c, nil
}

func (d *DB) GetCategories(ctx context.Context, userID int64) ([]models.Category, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT id, user_id, group_id, name, emoji, created_at
		FROM categories WHERE user_id = $1 ORDER BY name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get categories: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Category])
}

func (d *DB) DeleteCategory(ctx context.Context, userID, categoryID int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM categories WHERE id = $1 AND user_id = $2`, categoryID, userID)
	return err
}

// ─── Transactions ──────────────────────────────────────────

func (d *DB) CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*models.Transaction, error) {
	t := &models.Transaction{}
	// Serialize transaction timestamps with budget rollover. clock_timestamp is
	// evaluated after a waiting advisory lock is acquired, so a delayed write
	// cannot be backdated into a period the rollover has already closed.
	err := d.Pool.QueryRow(ctx, `
		WITH user_lock AS (
			SELECT pg_advisory_xact_lock($1)
		)
		INSERT INTO transactions (user_id, account_id, category_id, type, amount, currency, transfer_account_id, description, created_at)
		SELECT $1, source.id, $3, $4, $5, source.currency, $6, $7, clock_timestamp()
		FROM accounts source
		CROSS JOIN user_lock
		LEFT JOIN accounts target ON target.id = $6 AND target.user_id = $1
		WHERE source.id = $2
			AND source.user_id = $1
			AND ($3::integer IS NULL OR EXISTS (
				SELECT 1 FROM categories c WHERE c.id = $3 AND c.user_id = $1
			))
			AND ($4 <> 'transfer' OR (target.id IS NOT NULL AND target.currency = source.currency))
		RETURNING id, user_id, account_id, category_id, type, amount, currency, transfer_account_id, description, created_at
	`, userID, accountID, categoryID, txType, amount, transferAccountID, description).Scan(
		&t.ID, &t.UserID, &t.AccountID, &t.CategoryID, &t.Type, &t.Amount, &t.Currency, &t.TransferAccountID, &t.Description, &t.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("account/category ownership mismatch or unsupported cross-currency transfer")
	}
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	return t, nil
}

func (d *DB) GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]models.Transaction, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.user_id, t.account_id, t.category_id, t.type, t.amount, t.currency, t.transfer_account_id, t.description, t.created_at,
			COALESCE(c.name, '') AS category_name, COALESCE(c.emoji, '') AS category_emoji, COALESCE(a.name, '') AS account_name
		FROM transactions t
		LEFT JOIN categories c ON c.id = t.category_id
		LEFT JOIN accounts a ON a.id = t.account_id
		WHERE t.user_id = $1
		ORDER BY t.created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent transactions: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Transaction])
}

// ─── Budgets ──────────────────────────────────────────────

func (d *DB) SetBudget(ctx context.Context, userID int64, categoryID int64, currency string, intervalDays, intervalMonths int, amount float64) (*models.Budget, error) {
	currency, err := models.NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}
	if err := models.ValidateBudgetInterval(intervalDays, intervalMonths); err != nil {
		return nil, err
	}
	b := &models.Budget{}
	err = d.Pool.QueryRow(ctx, `
		INSERT INTO budgets (user_id, category_id, currency, period_start, interval_days, interval_months, amount)
		SELECT $1, c.id, UPPER($3), NOW(), $4, $5, $6
		FROM categories c
		WHERE c.id = $2 AND c.user_id = $1
		ON CONFLICT (user_id, category_id, currency) DO UPDATE
			SET interval_days   = EXCLUDED.interval_days,
			    interval_months = EXCLUDED.interval_months,
			    amount          = EXCLUDED.amount
		RETURNING id, user_id, category_id, currency, period_start, interval_days, interval_months, amount, rollover, created_at
	`, userID, categoryID, currency, intervalDays, intervalMonths, amount).Scan(
		&b.ID, &b.UserID, &b.CategoryID, &b.Currency, &b.PeriodStart, &b.IntervalDays, &b.IntervalMonths, &b.Amount, &b.Rollover, &b.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("category %d does not belong to user", categoryID)
	}
	if err != nil {
		return nil, fmt.Errorf("set budget: %w", err)
	}
	return b, nil
}

// GetBudgetSummary returns categories with spent/budget/available/remaining for the current period.
// Automatically rolls over expired budgets: unspent money carries forward to the next period.
func (d *DB) GetBudgetSummary(ctx context.Context, userID int64) ([]models.BudgetRow, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin budget summary: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return nil, fmt.Errorf("lock budget summary: %w", err)
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, fmt.Errorf("get database time: %w", err)
	}

	expiredRows, err := tx.Query(ctx, `
		SELECT id, user_id, category_id, currency, period_start,
			interval_days, interval_months, amount, rollover, created_at
		FROM budgets
		WHERE user_id = $1
			AND $2::timestamptz >= period_start + make_interval(days => interval_days, months => interval_months)
		FOR UPDATE
	`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("lock expired budgets: %w", err)
	}
	expired, err := pgx.CollectRows(expiredRows, pgx.RowToStructByName[models.Budget])
	if err != nil {
		return nil, fmt.Errorf("collect expired budgets: %w", err)
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
				SELECT $1::timestamptz + make_interval(days => $2, months => $3)
			`, periodStart, budget.IntervalDays, budget.IntervalMonths).Scan(&periodEnd); err != nil {
				return nil, fmt.Errorf("calculate budget period end: %w", err)
			}
			if now.Before(periodEnd) {
				break
			}

			var spent float64
			if err := tx.QueryRow(ctx, `
				SELECT COALESCE(SUM(amount), 0)
				FROM transactions
				WHERE user_id = $1 AND category_id = $2 AND currency = $3
					AND type = 'expense' AND created_at >= $4 AND created_at < $5
			`, userID, budget.CategoryID, budget.Currency, periodStart, periodEnd).Scan(&spent); err != nil {
				return nil, fmt.Errorf("calculate budget rollover: %w", err)
			}
			rollover = math.Max(0, budget.Amount+rollover-spent)
			periodStart = periodEnd
		}

		if _, err := tx.Exec(ctx, `
			UPDATE budgets SET rollover = $1, period_start = $2
			WHERE id = $3 AND user_id = $4
		`, rollover, periodStart, budget.ID, userID); err != nil {
			return nil, fmt.Errorf("update budget rollover: %w", err)
		}
	}

	rows, err := tx.Query(ctx, `
		WITH currency_set AS (
			SELECT b.category_id, b.currency
			FROM budgets b
			WHERE b.user_id = $1
			UNION
			SELECT t.category_id, t.currency
			FROM transactions t
			WHERE t.user_id = $1
				AND t.category_id IS NOT NULL
				AND t.type = 'expense'
				AND t.created_at >= date_trunc('month', $2::timestamptz)
			UNION
			SELECT c.id, 'EUR'
			FROM categories c
			WHERE c.user_id = $1
				AND NOT EXISTS (SELECT 1 FROM budgets b WHERE b.user_id = $1 AND b.category_id = c.id)
				AND NOT EXISTS (
					SELECT 1 FROM transactions t
					WHERE t.user_id = $1 AND t.category_id = c.id AND t.type = 'expense'
						AND t.created_at >= date_trunc('month', $2::timestamptz)
				)
		)
		SELECT
			c.id, c.user_id, c.name, c.emoji, cs.currency, c.created_at,
			COALESCE(SUM(t.amount), 0) AS spent,
			COALESCE(b.amount, 0) AS budget,
			COALESCE(b.rollover, 0) AS rollover,
			COALESCE(b.amount, 0) + COALESCE(b.rollover, 0) AS available,
			(COALESCE(b.amount, 0) + COALESCE(b.rollover, 0)) - COALESCE(SUM(t.amount), 0) AS remaining,
			COALESCE(g.name, '') AS group_name,
			c.group_id
		FROM currency_set cs
		JOIN categories c ON c.id = cs.category_id AND c.user_id = $1
		LEFT JOIN budgets b ON b.category_id = c.id AND b.user_id = $1 AND b.currency = cs.currency
		LEFT JOIN transactions t ON t.category_id = c.id
			AND t.user_id = $1
			AND t.type = 'expense'
			AND t.currency = cs.currency
			AND t.created_at >= COALESCE(b.period_start, date_trunc('month', $2::timestamptz))
		LEFT JOIN category_groups g ON g.id = c.group_id AND g.user_id = c.user_id
		GROUP BY c.id, cs.currency, b.amount, b.rollover, g.name, c.group_id
		ORDER BY COALESCE(g.name, 'zzz'), c.name, cs.currency
	`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("get budget summary: %w", err)
	}
	result, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.BudgetRow])
	if err != nil {
		return nil, fmt.Errorf("collect budget summary: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit budget summary: %w", err)
	}
	return result, nil
}

// GetBudgets returns all budgets for a user.
func (d *DB) GetBudgets(ctx context.Context, userID int64) ([]models.Budget, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT b.id, b.user_id, b.category_id, b.currency, b.period_start, b.interval_days, b.interval_months, b.amount, b.rollover, b.created_at
		FROM budgets b WHERE b.user_id = $1 ORDER BY b.category_id, b.currency
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get budgets: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Budget])
}

// GetReadyToAssign returns income minus assigned budgets independently for each currency.
func (d *DB) GetReadyToAssign(ctx context.Context, userID int64) ([]models.CurrencyAmount, error) {
	rows, err := d.Pool.Query(ctx, `
		WITH currencies AS (
			SELECT currency FROM transactions WHERE user_id = $1 AND type = 'income'
			UNION
			SELECT currency FROM budgets WHERE user_id = $1
		)
		SELECT c.currency,
			COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.user_id = $1 AND t.type = 'income' AND t.currency = c.currency), 0)
			- COALESCE((SELECT SUM(b.amount) FROM budgets b WHERE b.user_id = $1 AND b.currency = c.currency), 0) AS amount
		FROM currencies c
		ORDER BY c.currency
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get ready to assign: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.CurrencyAmount])
}
