package models

import "context"

// Store is the data layer for the bot. Backed by PostgreSQL in production,
// by an in-memory map in tests.
type Store interface {
	// Users
	GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*User, error)

	// Accounts
	CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*Account, error)
	GetAccounts(ctx context.Context, userID int64) ([]Account, error)
	GetAccount(ctx context.Context, id int64) (*Account, error)

	// Categories
	CreateCategory(ctx context.Context, userID int64, name, emoji string) (*Category, error)
	GetCategories(ctx context.Context, userID int64) ([]Category, error)
	DeleteCategory(ctx context.Context, categoryID int64) error

	// Transactions
	CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*Transaction, error)
	GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]Transaction, error)

	// Budgets
	SetBudget(ctx context.Context, userID int64, categoryID int64, month string, amount float64) (*Budget, error)
	GetBudgetSummary(ctx context.Context, userID int64) ([]Category, error)
	GetBudgets(ctx context.Context, userID int64) ([]Budget, error)
}
