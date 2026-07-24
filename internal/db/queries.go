package db

import (
	"context"
	"fmt"
	"time"

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

// ─── Accounts ─────────────────────────────────────────────

func (d *DB) CreateAccount(ctx context.Context, userID int64, name, accType, currency string, initialBalance float64) (*models.Account, error) {
	a := &models.Account{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO accounts (user_id, name, type, currency, initial_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, name, type, currency, initial_balance, created_at
	`, userID, name, accType, currency, initialBalance).Scan(
		&a.ID, &a.UserID, &a.Name, &a.Type, &a.Currency, &a.InitialBalance, &a.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return a, nil
}

func (d *DB) GetAccounts(ctx context.Context, userID int64) ([]models.Account, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT a.id, a.user_id, a.name, a.type, a.currency, a.initial_balance, a.created_at,
			COALESCE(a.initial_balance, 0) +
			COALESCE(SUM(CASE WHEN t.type = 'income' THEN t.amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN t.type = 'expense' THEN t.amount ELSE 0 END), 0) +
			COALESCE(SUM(CASE WHEN t.type = 'transfer' AND t.transfer_account_id = a.id THEN t.amount ELSE 0 END), 0) -
			COALESCE(SUM(CASE WHEN t.type = 'transfer' AND t.account_id = a.id THEN t.amount ELSE 0 END), 0) AS balance
		FROM accounts a
		LEFT JOIN transactions t ON (t.account_id = a.id OR t.transfer_account_id = a.id)
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
		SELECT id, user_id, name, type, currency, initial_balance, created_at
		FROM accounts WHERE id = $1
	`, id).Scan(&a.ID, &a.UserID, &a.Name, &a.Type, &a.Currency, &a.InitialBalance, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return a, nil
}

// ─── Categories ───────────────────────────────────────────

func (d *DB) CreateCategory(ctx context.Context, userID int64, name, emoji string) (*models.Category, error) {
	c := &models.Category{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO categories (user_id, name, emoji)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, emoji, created_at
	`, userID, name, emoji).Scan(&c.ID, &c.UserID, &c.Name, &c.Emoji, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return c, nil
}

func (d *DB) GetCategories(ctx context.Context, userID int64) ([]models.Category, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT id, user_id, name, emoji, created_at
		FROM categories
		WHERE user_id = $1
		ORDER BY name
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

// ─── Transactions ─────────────────────────────────────────

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
			COALESCE(c.name, '') AS category_name,
			COALESCE(c.emoji, '') AS category_emoji,
			a.name AS account_name
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id
		LEFT JOIN categories c ON c.id = t.category_id
		WHERE t.user_id = $1
		ORDER BY t.created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get transactions: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Transaction])
}

// ─── Budgets ──────────────────────────────────────────────

func (d *DB) SetBudget(ctx context.Context, userID int64, categoryID int64, month string, amount float64) (*models.Budget, error) {
	b := &models.Budget{}
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO budgets (user_id, category_id, month, amount)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, category_id, month) DO UPDATE
			SET amount = EXCLUDED.amount
		RETURNING id, user_id, category_id, month, amount, created_at
	`, userID, categoryID, month, amount).Scan(
		&b.ID, &b.UserID, &b.CategoryID, &b.Month, &b.Amount, &b.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("set budget: %w", err)
	}
	return b, nil
}

// GetBudgetSummary returns categories with spent/budget/remaining for current month.
func (d *DB) GetBudgetSummary(ctx context.Context, userID int64) ([]models.Category, error) {
	month := time.Now().Format("2006-01") + "-01"
	rows, err := d.Pool.Query(ctx, `
		SELECT
			c.id, c.user_id, c.name, c.emoji, c.created_at,
			COALESCE(SUM(CASE WHEN t.type = 'expense' THEN t.amount ELSE 0 END), 0) AS spent,
			COALESCE(b.amount, 0) AS budget,
			COALESCE(b.amount, 0) - COALESCE(SUM(CASE WHEN t.type = 'expense' THEN t.amount ELSE 0 END), 0) AS remaining
		FROM categories c
		LEFT JOIN transactions t ON t.category_id = c.id
			AND t.user_id = $1
			AND t.type = 'expense'
			AND DATE_TRUNC('month', t.created_at) = DATE_TRUNC('month', $2::date)
		LEFT JOIN budgets b ON b.category_id = c.id
			AND b.user_id = $1
			AND b.month = $2::date
		WHERE c.user_id = $1
		GROUP BY c.id, b.amount
		ORDER BY c.name
	`, userID, month)
	if err != nil {
		return nil, fmt.Errorf("get budget summary: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Category])
}

// GetBudgets returns all budgets for a user.
func (d *DB) GetBudgets(ctx context.Context, userID int64) ([]models.Budget, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT b.id, b.user_id, b.category_id, b.month, b.amount, b.created_at
		FROM budgets b
		WHERE b.user_id = $1
		ORDER BY b.month DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get budgets: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Budget])
}
