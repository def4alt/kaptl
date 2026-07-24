package bot

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/def4alt/kaptl/internal/models"
)

// memStore is an in-memory implementation of models.Store for testing.
type memStore struct {
	mu           sync.Mutex
	users        map[int64]*models.User
	accounts     map[int64]*models.Account
	categories   map[int64]*models.Category
	transactions []*models.Transaction
	budgets      map[string]*models.Budget // key: "userID|categoryID|month"

	nextAccountID  int64
	nextCategoryID int64
	nextTxID       int64
	nextBudgetID   int64
}

func newMemStore() *memStore {
	return &memStore{
		users:      make(map[int64]*models.User),
		accounts:   make(map[int64]*models.Account),
		categories: make(map[int64]*models.Category),
		budgets:    make(map[string]*models.Budget),
	}
}

func (m *memStore) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[telegramID]; ok {
		u.Username = username
		u.FirstName = firstName
		return u, nil
	}
	u := &models.User{
		TelegramID:   telegramID,
		Username:     username,
		FirstName:    firstName,
		LanguageCode: lang,
		CreatedAt:    time.Now(),
	}
	m.users[telegramID] = u
	return u, nil
}

func (m *memStore) CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextAccountID++
	a := &models.Account{
		ID:             m.nextAccountID,
		UserID:         userID,
		Name:           name,
		Emoji:          emoji,
		Currency:       currency,
		InitialBalance: initialBalance,
		CreatedAt:      time.Now(),
	}
	m.accounts[a.ID] = a
	return a, nil
}

func (m *memStore) GetAccounts(ctx context.Context, userID int64) ([]models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.Account
	for _, a := range m.accounts {
		if a.UserID == userID {
			// Compute balance from transactions
			balance := a.InitialBalance
			for _, t := range m.transactions {
				if t.AccountID == a.ID {
					switch t.Type {
					case "income":
						balance += t.Amount
					case "expense":
						balance -= t.Amount
					}
				}
				if t.TransferAccountID != nil && *t.TransferAccountID == a.ID {
					balance += t.Amount
				}
			}
			ac := *a
			ac.Balance = balance
			result = append(result, ac)
		}
	}
	return result, nil
}

func (m *memStore) GetAccount(ctx context.Context, id int64) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	if !ok {
		return nil, fmt.Errorf("account %d not found", id)
	}
	return a, nil
}

func (m *memStore) CreateCategory(ctx context.Context, userID int64, name, emoji string) (*models.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Check for duplicate
	for _, c := range m.categories {
		if c.UserID == userID && c.Name == name {
			return nil, fmt.Errorf("category already exists")
		}
	}
	m.nextCategoryID++
	c := &models.Category{
		ID:        m.nextCategoryID,
		UserID:    userID,
		Name:      name,
		Emoji:     emoji,
		CreatedAt: time.Now(),
	}
	m.categories[c.ID] = c
	return c, nil
}

func (m *memStore) GetCategories(ctx context.Context, userID int64) ([]models.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.Category
	for _, c := range m.categories {
		if c.UserID == userID {
			result = append(result, *c)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *memStore) DeleteCategory(ctx context.Context, categoryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.categories, categoryID)
	return nil
}

func (m *memStore) CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount float64, transferAccountID *int64, description string) (*models.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextTxID++
	t := &models.Transaction{
		ID:                m.nextTxID,
		UserID:            userID,
		AccountID:         accountID,
		CategoryID:        categoryID,
		Type:              txType,
		Amount:            amount,
		TransferAccountID: transferAccountID,
		Description:       description,
		CreatedAt:         time.Now(),
	}
	m.transactions = append(m.transactions, t)
	return t, nil
}

func (m *memStore) GetRecentTransactions(ctx context.Context, userID int64, limit int) ([]models.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.Transaction
	for i := len(m.transactions) - 1; i >= 0; i-- {
		t := m.transactions[i]
		if t.UserID == userID {
			tx := *t
			// Fill in joined fields
			if tx.CategoryID != nil {
				if c, ok := m.categories[*tx.CategoryID]; ok {
					tx.CategoryName = c.Name
					tx.CategoryEmoji = c.Emoji
				}
			}
			if a, ok := m.accounts[tx.AccountID]; ok {
				tx.AccountName = a.Name
			}
			result = append(result, tx)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *memStore) SetBudget(ctx context.Context, userID int64, categoryID int64, month string, amount float64) (*models.Budget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d|%d|%s", userID, categoryID, month)
	if b, ok := m.budgets[key]; ok {
		b.Amount = amount
		return b, nil
	}
	m.nextBudgetID++
	b := &models.Budget{
		ID:         m.nextBudgetID,
		UserID:     userID,
		CategoryID: categoryID,
		Month:      month,
		Amount:     amount,
		CreatedAt:  time.Now(),
	}
	m.budgets[key] = b
	return b, nil
}

func (m *memStore) GetBudgetSummary(ctx context.Context, userID int64) ([]models.BudgetRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	month := time.Now().Format("2006-01") + "-01"
	var result []models.BudgetRow

	for _, c := range m.categories {
		if c.UserID != userID {
			continue
		}
		row := models.BudgetRow{
			ID:        c.ID,
			UserID:    c.UserID,
			Name:      c.Name,
			Emoji:     c.Emoji,
			CreatedAt: c.CreatedAt,
		}
		for _, t := range m.transactions {
			if t.UserID == userID && t.CategoryID != nil && *t.CategoryID == c.ID &&
				t.Type == "expense" && t.CreatedAt.Format("2006-01") == month[:7] {
				row.Spent += t.Amount
			}
		}
		key := fmt.Sprintf("%d|%d|%s", userID, c.ID, month)
		if b, ok := m.budgets[key]; ok {
			row.Budget = b.Amount
		}
		row.Remaining = row.Budget - row.Spent
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *memStore) GetBudgets(ctx context.Context, userID int64) ([]models.Budget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.Budget
	for _, b := range m.budgets {
		if b.UserID == userID {
			result = append(result, *b)
		}
	}
	return result, nil
}
