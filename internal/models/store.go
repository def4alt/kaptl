package models

import "context"

type Store interface {
	GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*User, error)

	CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*Account, error)
	GetAccounts(ctx context.Context, userID int64) ([]Account, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)

	CreateCategory(ctx context.Context, userID int64, name, emoji string) (*Category, error)
	GetCategories(ctx context.Context, userID int64) ([]Category, error)
	DeleteCategory(ctx context.Context, categoryID int64) error

	CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*Transaction, error)
	GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]Transaction, error)

	// SetBudget upserts a recurring budget. intervalDays=7 means weekly, intervalMonths=1 means monthly.
	SetBudget(ctx context.Context, userID int64, categoryID int64, intervalDays, intervalMonths int, amount float64) (*Budget, error)
	GetBudgetSummary(ctx context.Context, userID int64) ([]BudgetRow, error)
	GetBudgets(ctx context.Context, userID int64) ([]Budget, error)
}
