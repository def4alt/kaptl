package bot

import (
	"context"

	"github.com/def4alt/kaptl/internal/models"
)

// Store is the persistence contract required by the Telegram application.
// Implementations live outside this package and satisfy it implicitly.
type Store interface {
	GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*models.User, error)

	CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*models.Account, error)
	GetAccounts(ctx context.Context, userID int64) ([]models.Account, error)
	GetAccount(ctx context.Context, userID, id int64) (*models.Account, error)

	CreateGroup(ctx context.Context, userID int64, name, emoji string) (*models.CategoryGroup, error)
	GetGroups(ctx context.Context, userID int64) ([]models.CategoryGroup, error)
	DeleteGroup(ctx context.Context, userID, groupID int64) error

	CreateCategory(ctx context.Context, userID int64, name, emoji string, groupID *int64) (*models.Category, error)
	GetCategories(ctx context.Context, userID int64) ([]models.Category, error)
	DeleteCategory(ctx context.Context, userID, categoryID int64) error

	CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*models.Transaction, error)
	GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]models.Transaction, error)

	SetBudget(ctx context.Context, userID, categoryID int64, currency string, intervalDays, intervalMonths int, amount float64) (*models.Budget, error)
	GetBudgetSummary(ctx context.Context, userID int64) ([]models.BudgetRow, error)
	GetBudgets(ctx context.Context, userID int64) ([]models.Budget, error)
	GetReadyToAssign(ctx context.Context, userID int64) ([]models.CurrencyAmount, error)
}
