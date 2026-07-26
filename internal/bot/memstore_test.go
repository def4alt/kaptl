package bot

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

func d(value string) decimal.Decimal { return decimal.RequireFromString(value) }

type memStore struct {
	mu           sync.Mutex
	users        map[int64]*models.User
	accounts     map[int64]*models.Account
	groups       map[int64]*models.CategoryGroup
	categories   map[int64]*models.Category
	transactions []*models.Transaction
	budgets      map[string]*models.Budget
	valuations   map[int64]map[string]decimal.Decimal

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
		valuations: make(map[int64]map[string]decimal.Decimal),
	}
}

func (m *memStore) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lang string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[telegramID]; ok {
		return u, nil
	}
	u := &models.User{TelegramID: telegramID, Username: username, FirstName: firstName, LanguageCode: lang, ReportingCurrency: "EUR", CreatedAt: time.Now()}
	m.users[telegramID] = u
	return u, nil
}

func (m *memStore) CreateAccount(ctx context.Context, userID int64, name, emoji, currency string, initialBalance decimal.Decimal) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	currency, err := money.NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}
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
					case "income":
						balance = balance.Add(t.Amount)
					case "expense", "transfer":
						balance = balance.Sub(t.Amount)
					}
				}
				if t.TransferAccountID != nil && *t.TransferAccountID == a.ID {
					balance = balance.Add(t.Amount)
				}
			}
			ac := *a
			ac.Balance = balance
			result = append(result, ac)
		}
	}
	return result, nil
}

func (m *memStore) GetAccount(ctx context.Context, userID, id int64) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.accounts[id]
	if !ok || a.UserID != userID {
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

func (m *memStore) DeleteGroup(ctx context.Context, userID, groupID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	group, ok := m.groups[groupID]
	if !ok || group.UserID != userID {
		return fmt.Errorf("group not found")
	}
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
	if groupID != nil {
		group, ok := m.groups[*groupID]
		if !ok || group.UserID != userID {
			return nil, fmt.Errorf("group does not belong to user")
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

func (m *memStore) DeleteCategory(ctx context.Context, userID, categoryID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	category, ok := m.categories[categoryID]
	if !ok || category.UserID != userID {
		return fmt.Errorf("category not found")
	}
	delete(m.categories, categoryID)
	return nil
}

func (m *memStore) CreateTransaction(ctx context.Context, userID, accountID int64, categoryID *int64, txType string, amount decimal.Decimal, transferAccountID *int64, description string) (*models.Transaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.accounts[accountID]
	if !ok || account.UserID != userID {
		return nil, fmt.Errorf("account %d not found", accountID)
	}
	if txType == "transfer" {
		if transferAccountID == nil {
			return nil, fmt.Errorf("transfer account is required")
		}
		target, ok := m.accounts[*transferAccountID]
		if !ok || target.UserID != userID {
			return nil, fmt.Errorf("transfer account %d not found", *transferAccountID)
		}
		if target.Currency != account.Currency {
			return nil, fmt.Errorf("accounts use different currencies")
		}
	}
	m.nextTxID++
	t := &models.Transaction{ID: m.nextTxID, UserID: userID, AccountID: accountID, CategoryID: categoryID, Type: txType, Amount: amount, Currency: account.Currency, TransferAccountID: transferAccountID, Description: description, CreatedAt: time.Now()}
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
			if tx.Currency != "EUR" {
				if amount, ok := m.valuations[tx.ID]["EUR"]; ok {
					tx.ReportingAmount = &amount
					tx.ReportingCurrency = "EUR"
				}
			}
			result = append(result, tx)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *memStore) setValuation(transactionID int64, target string, amount decimal.Decimal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.valuations[transactionID] == nil {
		m.valuations[transactionID] = make(map[string]decimal.Decimal)
	}
	m.valuations[transactionID][target] = amount
}

func (m *memStore) SetBudget(ctx context.Context, userID int64, categoryID int64, intervalDays, intervalMonths int, amount decimal.Decimal) (*models.Budget, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	currency := "EUR"
	if err := models.ValidateBudgetInterval(intervalDays, intervalMonths); err != nil {
		return nil, err
	}
	category, ok := m.categories[categoryID]
	if !ok || category.UserID != userID {
		return nil, fmt.Errorf("category does not belong to user")
	}
	key := fmt.Sprintf("%d|%d|%s", userID, categoryID, currency)
	if b, ok := m.budgets[key]; ok {
		b.IntervalDays = intervalDays
		b.IntervalMonths = intervalMonths
		b.Amount = amount
		b.PeriodStart = time.Now()
		return b, nil
	}
	m.nextBudgetID++
	b := &models.Budget{ID: m.nextBudgetID, UserID: userID, CategoryID: categoryID, Currency: currency, PeriodStart: time.Now(), IntervalDays: intervalDays, IntervalMonths: intervalMonths, Amount: amount, CreatedAt: time.Now()}
	m.budgets[key] = b
	return b, nil
}

func (m *memStore) GetBudgetSummary(ctx context.Context, userID int64) ([]models.BudgetRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	monthStart := time.Now()
	monthStart = time.Date(monthStart.Year(), monthStart.Month(), 1, 0, 0, 0, 0, monthStart.Location())
	var result []models.BudgetRow
	for _, c := range m.categories {
		if c.UserID != userID {
			continue
		}

		currencies := make(map[string]bool)
		budgets := make(map[string]*models.Budget)
		for _, bd := range m.budgets {
			if bd.UserID == userID && bd.CategoryID == c.ID {
				currencies[bd.Currency] = true
				budgets[bd.Currency] = bd
			}
		}
		for _, tx := range m.transactions {
			if tx.UserID == userID && tx.CategoryID != nil && *tx.CategoryID == c.ID && tx.Type == "expense" && tx.CreatedAt.After(monthStart) {
				currencies["EUR"] = true
			}
		}
		if len(currencies) == 0 {
			currencies["EUR"] = true
		}

		for currency := range currencies {
			row := models.BudgetRow{ID: c.ID, UserID: c.UserID, Name: c.Name, Emoji: c.Emoji, Currency: currency, CreatedAt: c.CreatedAt, GroupID: c.GroupID}
			periodStart := monthStart
			if bd := budgets[currency]; bd != nil {
				row.Budget = bd.Amount
				row.Rollover = bd.Rollover
				row.Available = bd.Amount.Add(bd.Rollover)
				periodStart = bd.PeriodStart
			}
			for _, tx := range m.transactions {
				if tx.UserID != userID || tx.CategoryID == nil || *tx.CategoryID != c.ID || tx.Type != "expense" || !tx.CreatedAt.After(periodStart) {
					continue
				}
				if tx.Currency == currency {
					row.Spent = row.Spent.Add(tx.Amount)
				} else if amount, ok := m.valuations[tx.ID][currency]; ok {
					row.Spent = row.Spent.Add(amount)
				} else {
					row.MissingValuations++
				}
			}
			row.Remaining = row.Available.Sub(row.Spent)
			if c.GroupID != nil {
				if g, ok := m.groups[*c.GroupID]; ok {
					row.GroupName = g.Name
				}
			}
			result = append(result, row)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GroupName != result[j].GroupName {
			return result[i].GroupName < result[j].GroupName
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Currency < result[j].Currency
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

func (m *memStore) GetReadyToAssign(ctx context.Context, userID int64) ([]models.CurrencyAmount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	amount := decimal.Zero
	missing := 0
	for _, tx := range m.transactions {
		if tx.UserID != userID || tx.Type != "income" {
			continue
		}
		if tx.Currency == "EUR" {
			amount = amount.Add(tx.Amount)
		} else if valued, ok := m.valuations[tx.ID]["EUR"]; ok {
			amount = amount.Add(valued)
		} else {
			missing++
		}
	}
	for _, budget := range m.budgets {
		if budget.UserID == userID {
			amount = amount.Sub(budget.Amount)
		}
	}
	return []models.CurrencyAmount{{Currency: "EUR", Amount: amount, MissingValuations: missing}}, nil
}

func (m *memStore) GetReportingSummary(ctx context.Context, userID int64) (*models.ReportingSummary, error) {
	rows, err := m.GetBudgetSummary(ctx, userID)
	if err != nil {
		return nil, err
	}
	rta, err := m.GetReadyToAssign(ctx, userID)
	if err != nil {
		return nil, err
	}
	pending := 0
	for _, row := range rows {
		pending += row.MissingValuations
	}
	if len(rta) > 0 {
		pending += rta[0].MissingValuations
	}
	if pending > 0 {
		return nil, &models.ValuationsPendingError{Count: pending}
	}
	return &models.ReportingSummary{Currency: "EUR", Rows: rows, ReadyToAssign: rta}, nil
}

func TestMemStoreCreateCategoryRejectsAnotherUsersGroup(t *testing.T) {
	store := newMemStore()
	group, _ := store.CreateGroup(nil, 2, "Private", "🔒")

	if _, err := store.CreateCategory(nil, 1, "Food", "🍞", &group.ID); err == nil {
		t.Fatal("CreateCategory accepted another user's group")
	}
}

func TestMemStoreSetBudgetRejectsUnsafeIntervals(t *testing.T) {
	store := newMemStore()
	category, _ := store.CreateCategory(nil, 1, "Food", "🍞", nil)
	if _, err := store.SetBudget(nil, 2, category.ID, 0, 1, d("100")); err == nil {
		t.Fatal("SetBudget accepted another user's category")
	}
	tests := []struct {
		name   string
		days   int
		months int
	}{
		{name: "negative days", days: -1, months: 1},
		{name: "negative months", days: 1, months: -1},
		{name: "zero", days: 0, months: 0},
		{name: "days too large", days: 3651, months: 0},
		{name: "months too large", days: 0, months: 121},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.SetBudget(nil, 1, category.ID, tt.days, tt.months, d("100")); err == nil {
				t.Fatalf("SetBudget accepted interval days=%d months=%d", tt.days, tt.months)
			}
		})
	}
}

func TestMemStoreDeleteScopesObjectsToUser(t *testing.T) {
	store := newMemStore()
	group, _ := store.CreateGroup(nil, 1, "Private", "🔒")
	category, _ := store.CreateCategory(nil, 1, "Food", "🍞", &group.ID)

	if err := store.DeleteCategory(nil, 2, category.ID); err == nil {
		t.Fatal("DeleteCategory accepted another user's category")
	}
	if _, ok := store.categories[category.ID]; !ok {
		t.Fatal("DeleteCategory removed another user's category")
	}
	if err := store.DeleteGroup(nil, 2, group.ID); err == nil {
		t.Fatal("DeleteGroup accepted another user's group")
	}
	if _, ok := store.groups[group.ID]; !ok {
		t.Fatal("DeleteGroup removed another user's group")
	}
}
