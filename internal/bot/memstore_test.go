package bot

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/def4alt/kaptl/internal/models"
)

type memStore struct {
	mu           sync.Mutex
	users        map[int64]*models.User
	accounts     map[int64]*models.Account
	groups       map[int64]*models.CategoryGroup
	categories   map[int64]*models.Category
	transactions []*models.Transaction
	budgets      map[string]*models.Budget

	nextAccountID  int64
	nextGroupID    int64
	nextCategoryID int64
	nextTxID       int64
	nextBudgetID   int64
}

func newMemStore() *memStore {
	return &memStore{
		users:      make(map[int64]*models.User),
		accounts:   make(map[int64]*models.Account),
		groups:     make(map[int64]*models.CategoryGroup),
		categories: make(map[int64]*models.Category),
		budgets:    make(map[string]*models.Budget),
	}
}

func (m *memStore) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[telegramID]; ok {
		return u, nil
	}
	u := &models.User{TelegramID: telegramID, Username: username, FirstName: firstName, LanguageCode: lang, CreatedAt: time.Now()}
	m.users[telegramID] = u
	return u, nil
}

func (m *memStore) CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance float64) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextAccountID++
	a := &models.Account{ID: m.nextAccountID, UserID: userID, Name: name, Emoji: emoji, Currency: currency, InitialBalance: initialBalance, CreatedAt: time.Now()}
	m.accounts[a.ID] = a
	return a, nil
}

func (m *memStore) GetAccounts(ctx context.Context, userID int64) ([]models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.Account
	for _, a := range m.accounts {
		if a.UserID == userID {
			balance := a.InitialBalance
			for _, t := range m.transactions {
				if t.AccountID == a.ID {
					switch t.Type {
					case "income": balance += t.Amount
					case "expense", "transfer": balance -= t.Amount
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

func (m *memStore) CreateGroup(ctx context.Context, userID int64, name, emoji string) (*models.CategoryGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextGroupID++
	g := &models.CategoryGroup{ID: m.nextGroupID, UserID: userID, Name: name, Emoji: emoji, CreatedAt: time.Now()}
	m.groups[g.ID] = g
	return g, nil
}

func (m *memStore) GetGroups(ctx context.Context, userID int64) ([]models.CategoryGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.CategoryGroup
	for _, g := range m.groups {
		if g.UserID == userID {
			result = append(result, *g)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SortOrder < result[j].SortOrder })
	return result, nil
}

func (m *memStore) DeleteGroup(ctx context.Context, groupID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, groupID)
	for _, c := range m.categories {
		if c.GroupID != nil && *c.GroupID == groupID {
			c.GroupID = nil
		}
	}
	return nil
}

func (m *memStore) CreateCategory(ctx context.Context, userID int64, name, emoji string, groupID *int64) (*models.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.categories {
		if c.UserID == userID && c.Name == name {
			return nil, fmt.Errorf("category already exists")
		}
	}
	m.nextCategoryID++
	c := &models.Category{ID: m.nextCategoryID, UserID: userID, GroupID: groupID, Name: name, Emoji: emoji, CreatedAt: time.Now()}
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
	t := &models.Transaction{ID: m.nextTxID, UserID: userID, AccountID: accountID, CategoryID: categoryID, Type: txType, Amount: amount, TransferAccountID: transferAccountID, Description: description, CreatedAt: time.Now()}
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

func (m *memStore) SetBudget(ctx context.Context, userID int64, categoryID int64, intervalDays, intervalMonths int, amount float64) (*models.Budget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%d|%d", userID, categoryID)
	if b, ok := m.budgets[key]; ok {
		b.IntervalDays = intervalDays
		b.IntervalMonths = intervalMonths
		b.Amount = amount
		b.PeriodStart = time.Now()
		return b, nil
	}
	m.nextBudgetID++
	b := &models.Budget{ID: m.nextBudgetID, UserID: userID, CategoryID: categoryID, PeriodStart: time.Now(), IntervalDays: intervalDays, IntervalMonths: intervalMonths, Amount: amount, CreatedAt: time.Now()}
	m.budgets[key] = b
	return b, nil
}

func (m *memStore) GetBudgetSummary(ctx context.Context, userID int64, periodOffset int) ([]models.BudgetRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = periodOffset // not used in memStore — always current

	var result []models.BudgetRow
	for _, c := range m.categories {
		if c.UserID != userID {
			continue
		}
		row := models.BudgetRow{ID: c.ID, UserID: c.UserID, Name: c.Name, Emoji: c.Emoji, CreatedAt: c.CreatedAt, GroupID: c.GroupID}
		key := fmt.Sprintf("%d|%d", userID, c.ID)
		if bd, ok := m.budgets[key]; ok {
			row.Budget = bd.Amount
			row.Rollover = bd.Rollover
			row.Available = bd.Amount + bd.Rollover
			for _, t := range m.transactions {
				if t.UserID == userID && t.CategoryID != nil && *t.CategoryID == c.ID && t.Type == "expense" && t.CreatedAt.After(bd.PeriodStart) {
					row.Spent += t.Amount
				}
			}
		}
		row.Remaining = row.Available - row.Spent
		if c.GroupID != nil {
			if g, ok := m.groups[*c.GroupID]; ok {
				row.GroupName = g.Name
			}
		}
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GroupName != result[j].GroupName {
			return result[i].GroupName < result[j].GroupName
		}
		return result[i].Name < result[j].Name
	})
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

func (m *memStore) GetReadyToAssign(ctx context.Context, userID int64) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var income float64
	var assigned float64
	for _, t := range m.transactions {
		if t.UserID == userID && t.Type == "income" {
			income += t.Amount
		}
	}
	for _, b := range m.budgets {
		if b.UserID == userID {
			assigned += b.Amount
		}
	}
	return income - assigned, nil
}
