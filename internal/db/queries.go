package db

import (
	"context"
	"fmt"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/jackc/pgx/v5"
)

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
	a := &models.Account{}
	err := d.Pool.QueryRow(ctx, `
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

func (d *DB) GetAccount(ctx context.Context, id int64) (*models.Account, error) {
	a := &models.Account{}
	err := d.Pool.QueryRow(ctx, `
		SELECT id, user_id, name, emoji, currency, initial_balance, created_at
		FROM accounts WHERE id = $1
	`, id).Scan(&a.ID, &a.UserID, &a.Name, &a.Emoji, &a.Currency, &a.InitialBalance, &a.CreatedAt)
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

func (d *DB) DeleteGroup(ctx context.Context, groupID int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM category_groups WHERE id = $1`, groupID)
	return err
}

// ─── Categories ────────────────────────────────────────────

func (d *DB) CreateCategory(ctx context.Context, userID int64, name, emoji string, groupID *int64) (*models.Category, error) {
	c := &models.Category{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO categories (user_id, name, emoji, group_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, group_id, name, emoji, created_at
	`, userID, name, emoji, groupID).Scan(
		&c.ID, &c.UserID, &c.GroupID, &c.Name, &c.Emoji, &c.CreatedAt,
	)
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

func (d *DB) DeleteCategory(ctx context.Context, categoryID int64) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, categoryID)
	return err
}

// ─── Transactions ──────────────────────────────────────────

func (d *DB) CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*models.Transaction, error) {
	t := &models.Transaction{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO transactions (user_id, account_id, category_id, type, amount, transfer_account_id, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, account_id, category_id, type, amount, transfer_account_id, description, created_at
	`, userID, accountID, categoryID, txType, amount, transferAccountID, description).Scan(
		&t.ID, &t.UserID, &t.AccountID, &t.CategoryID, &t.Type, &t.Amount, &t.TransferAccountID, &t.Description, &t.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}
	return t, nil
}

func (d *DB) GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]models.Transaction, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT t.id, t.user_id, t.account_id, t.category_id, t.type, t.amount, t.transfer_account_id, t.description, t.created_at,
			COALESCE(c.name, ''), COALESCE(c.emoji, ''), COALESCE(a.name, '')
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

func (d *DB) SetBudget(ctx context.Context, userID int64, categoryID int64, intervalDays, intervalMonths int, amount float64) (*models.Budget, error) {
	b := &models.Budget{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO budgets (user_id, category_id, period_start, interval_days, interval_months, amount)
		VALUES ($1, $2, NOW(), $3, $4, $5)
		ON CONFLICT (user_id, category_id) DO UPDATE
			SET interval_days   = EXCLUDED.interval_days,
			    interval_months = EXCLUDED.interval_months,
			    amount         = EXCLUDED.amount
		RETURNING id, user_id, category_id, period_start, interval_days, interval_months, amount, rollover, created_at
	`, userID, categoryID, intervalDays, intervalMonths, amount).Scan(
		&b.ID, &b.UserID, &b.CategoryID, &b.PeriodStart, &b.IntervalDays, &b.IntervalMonths, &b.Amount, &b.Rollover, &b.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("set budget: %w", err)
	}
	return b, nil
}

// GetBudgetSummary returns categories with spent/budget/available/remaining for the current period.
// Automatically rolls over expired budgets: unspent money carries forward to the next period.
func (d *DB) GetBudgetSummary(ctx context.Context, userID int64, periodOffset int) ([]models.BudgetRow, error) {
	// Step 1: Roll over any expired budgets — advance period_start and carry forward leftovers
	d.Pool.Exec(ctx, `
		WITH expired AS (
			SELECT b.id, b.amount, b.rollover,
				COALESCE(SUM(CASE WHEN t.type = 'expense' THEN t.amount ELSE 0 END), 0) AS spent,
				b.period_start + make_interval(days => b.interval_days, months => b.interval_months) AS next_start
			FROM budgets b
			LEFT JOIN transactions t ON t.category_id = b.category_id
				AND t.user_id = b.user_id
				AND t.type = 'expense'
				AND t.created_at >= b.period_start
			WHERE b.user_id = $1
				AND NOW() >= b.period_start + make_interval(days => b.interval_days, months => b.interval_months)
			GROUP BY b.id
		)
		UPDATE budgets b
		SET rollover = GREATEST(0, e.amount + e.rollover - e.spent),
		    period_start = e.next_start
		FROM expired e
		WHERE b.id = e.id
	`, userID)

	// Step 2: Return the summary
	rows, err := d.Pool.Query(ctx, `
		SELECT
			c.id, c.user_id, c.name, c.emoji, c.created_at,
			COALESCE(SUM(CASE WHEN t2.type = 'expense' THEN t2.amount ELSE 0 END), 0) AS spent,
			COALESCE(b.amount, 0) AS budget,
			COALESCE(b.rollover, 0) AS rollover,
			COALESCE(b.amount, 0) + COALESCE(b.rollover, 0) AS available,
			(COALESCE(b.amount, 0) + COALESCE(b.rollover, 0)) - COALESCE(SUM(CASE WHEN t2.type = 'expense' THEN t2.amount ELSE 0 END), 0) AS remaining,
			COALESCE(g.name, '') AS group_name,
			c.group_id
		FROM categories c
		LEFT JOIN budgets b ON b.category_id = c.id AND b.user_id = $1
		LEFT JOIN transactions t2 ON t2.category_id = c.id
			AND t2.user_id = $1
			AND t2.type = 'expense'
			AND t2.created_at >= COALESCE(b.period_start, '1970-01-01'::timestamptz)
		LEFT JOIN category_groups g ON g.id = c.group_id
		WHERE c.user_id = $1
		GROUP BY c.id, b.amount, b.rollover, g.name, c.group_id
		ORDER BY COALESCE(g.name, 'zzz'), c.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get budget summary: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.BudgetRow])
}

// GetBudgets returns all budgets for a user.
func (d *DB) GetBudgets(ctx context.Context, userID int64) ([]models.Budget, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT b.id, b.user_id, b.category_id, b.period_start, b.interval_days, b.interval_months, b.amount, b.rollover, b.created_at
		FROM budgets b WHERE b.user_id = $1 ORDER BY b.category_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get budgets: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Budget])
}

// GetReadyToAssign returns how much money is available to budget.
// Formula: total income - total budget amounts assigned.
func (d *DB) GetReadyToAssign(ctx context.Context, userID int64) (float64, error) {
	var rta float64
	err := d.Pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0)
			- COALESCE((SELECT SUM(amount) FROM budgets WHERE user_id = $1), 0)
		FROM transactions WHERE user_id = $1
	`, userID).Scan(&rta)
	if err != nil {
		return 0, fmt.Errorf("get ready to assign: %w", err)
	}
	return rta, nil
}
