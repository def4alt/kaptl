package db

import (
	"context"
	"fmt"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
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
		RETURNING telegram_id, username, first_name, language_code, reporting_currency, created_at
	`, telegramID, username, firstName, lang).Scan(
		&u.TelegramID, &u.Username, &u.FirstName, &u.LanguageCode, &u.ReportingCurrency, &u.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get or create user: %w", err)
	}
	return u, nil
}

// ─── Accounts ──────────────────────────────────────────────

func (d *DB) CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance decimal.Decimal) (*models.Account, error) {
	currency, err := money.NormalizeCurrency(currency)
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
	_, err := d.Pool.Exec(ctx, `
		WITH user_lock AS MATERIALIZED (SELECT pg_advisory_xact_lock($2))
		DELETE FROM category_groups g USING user_lock
		WHERE g.id=$1 AND g.user_id=$2
	`, groupID, userID)
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
	_, err := d.Pool.Exec(ctx, `
		WITH user_lock AS MATERIALIZED (SELECT pg_advisory_xact_lock($2))
		DELETE FROM categories c USING user_lock
		WHERE c.id=$1 AND c.user_id=$2
	`, categoryID, userID)
	return err
}

// ─── Transactions ──────────────────────────────────────────

func (d *DB) CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount decimal.Decimal, transferAccountID *int64, description string) (*models.Transaction, error) {
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
			COALESCE(c.name, '') AS category_name, COALESCE(c.emoji, '') AS category_emoji, COALESCE(a.name, '') AS account_name,
			v.amount AS reporting_amount, COALESCE(v.target_currency, '') AS reporting_currency
		FROM transactions t
		LEFT JOIN categories c ON c.id = t.category_id
		LEFT JOIN accounts a ON a.id = t.account_id
		LEFT JOIN users u ON u.telegram_id = t.user_id
		LEFT JOIN transaction_valuations v ON v.transaction_id = t.id
			AND v.target_currency = u.reporting_currency AND v.purpose = 'budget' AND v.policy_version = 1
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

func (d *DB) SetBudget(ctx context.Context, userID int64, categoryID int64, intervalDays, intervalMonths int, amount decimal.Decimal) (*models.Budget, error) {
	if err := models.ValidateBudgetInterval(intervalDays, intervalMonths); err != nil {
		return nil, err
	}
	b := &models.Budget{}
	err := d.Pool.QueryRow(ctx, `
		WITH user_lock AS MATERIALIZED (SELECT pg_advisory_xact_lock($1))
		INSERT INTO budgets (user_id, category_id, currency, period_start, interval_days, interval_months, amount)
		SELECT $1, c.id, u.reporting_currency, clock_timestamp(), $3, $4, $5
		FROM categories c
		JOIN users u ON u.telegram_id = $1
		CROSS JOIN user_lock
		WHERE c.id = $2 AND c.user_id = $1
		ON CONFLICT (user_id, category_id, currency) DO UPDATE
			SET interval_days   = EXCLUDED.interval_days,
			    interval_months = EXCLUDED.interval_months,
			    amount          = EXCLUDED.amount
		RETURNING id, user_id, category_id, currency, period_start, interval_days, interval_months, amount, rollover, created_at
	`, userID, categoryID, intervalDays, intervalMonths, amount).Scan(
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
