package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/def4alt/kaptl/internal/db"
	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v3"
)

// ─── Wizard state (conversation-like step tracking) ──────

type userState struct {
	Step          string // "awaiting_amount", "awaiting_account", etc.
	CategoryID    int64
	AccountID     int64
	Amount        float64
	Description   string
	TxType        string // "expense" or "income"
	EditingBudget int64  // category ID when setting budget
}

// ─── Bot ──────────────────────────────────────────────────

type Bot struct {
	Tele   *tele.Bot
	DB     *db.DB
	States map[int64]*userState // telegramID -> state
}

func New(database *db.DB) (*Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}
	allowedID, _ := strconv.ParseInt(os.Getenv("ALLOWED_TELEGRAM_ID"), 10, 64)

	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	tb, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	b := &Bot{
		Tele:   tb,
		DB:     database,
		States: make(map[int64]*userState),
	}

	// Auth middleware
	if allowedID != 0 {
		tb.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
			return func(c tele.Context) error {
				if c.Sender().ID != allowedID {
					return c.Send("⛔ This bot is private.")
				}
				return next(c)
			}
		})
	}

	// Ensure user exists
	tb.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			ctx := context.Background()
			sender := c.Sender()
			_, err := b.DB.GetOrCreateUser(ctx, sender.ID, sender.Username, sender.FirstName, sender.LanguageCode)
			if err != nil {
				log.Printf("get or create user: %v", err)
			}
			return next(c)
		}
	})

	b.registerHandlers()
	return b, nil
}

func (b *Bot) Start() {
	log.Println("🤖 Bot starting...")
	b.Tele.Start()
}

// ─── Handlers ─────────────────────────────────────────────

func (b *Bot) registerHandlers() {
	b.Tele.Handle("/start", b.handleStart)
	b.Tele.Handle("/menu", b.handleStart)
	b.Tele.Handle("/help", b.handleHelp)

	// Callback buttons
	b.Tele.Handle(&btnAddExpense, b.handleAddExpense)
	b.Tele.Handle(&btnAddIncome, b.handleAddIncome)
	b.Tele.Handle(&btnSummary, b.handleSummary)
	b.Tele.Handle(&btnAccounts, b.handleAccounts)
	b.Tele.Handle(&btnCategories, b.handleCategories)
	b.Tele.Handle(&btnBudgets, b.handleBudgets)
	b.Tele.Handle(&btnRecent, b.handleRecent)
	b.Tele.Handle(&btnCancel, b.handleCancel)

	// Dynamic callbacks (category/account/budget picks)
	b.Tele.Handle(tele.OnCallback, b.handleCallback)

	// Text input (amount, category name, budget amount, etc.)
	b.Tele.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleStart(c tele.Context) error {
	userID := c.Sender().ID
	delete(b.States, userID) // clear any wizard state

	return c.Send("💰 *YNAB Bot*\n\nTrack your expenses like a pro.", mainMenu())
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(`*Commands:*
/start – Main menu
/menu – Main menu

*Quick expense:*
Tap "➕ Expense" → pick category → type amount → pick account → done!

*Budgets:*
Use "🎯 Budgets" to set monthly limits per category.
Use "📊 Summary" to see where you stand.`, mainMenu())
}

// ─── Add Expense ──────────────────────────────────────────

func (b *Bot) handleAddExpense(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	cats, err := b.DB.GetCategories(ctx, userID)
	if err != nil || len(cats) == 0 {
		return c.Send("No categories yet! Use *🏷️ Categories* to create some first.", mainMenu())
	}

	return c.Send("*Pick a category:*", categoryKeyboard(cats, ""))
}

// ─── Add Income ───────────────────────────────────────────

func (b *Bot) handleAddIncome(c tele.Context) error {
	userID := c.Sender().ID
	b.States[userID] = &userState{Step: "awaiting_income_amount", TxType: "income"}

	return c.Send("*Enter the income amount:*\n\nJust type a number, e.g. `1500`", cancelBtn())
}

// ─── Summary ──────────────────────────────────────────────

func (b *Bot) handleSummary(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	cats, err := b.DB.GetBudgetSummary(ctx, userID)
	if err != nil {
		return c.Send("Error loading summary", mainMenu())
	}

	if len(cats) == 0 {
		return c.Send("No categories yet. Use *🏷️ Categories* to create some.", mainMenu())
	}

	totalBudget := 0.0
	totalSpent := 0.0

	var lines []string
	for _, cat := range cats {
		lines = append(lines, fmt.Sprintf("%s *%s*: %.0f / %.0f (%.0f left)",
			cat.Emoji, cat.Name, cat.Spent, cat.Budget, cat.Remaining))
		totalBudget += cat.Budget
		totalSpent += cat.Spent
	}

	msg := fmt.Sprintf("📊 *Budget Summary — %s*\n\n", time.Now().Format("January 2006"))
	msg += strings.Join(lines, "\n")
	msg += fmt.Sprintf("\n\n💵 Total: *%.0f / %.0f* (%.0f left)", totalSpent, totalBudget, totalBudget-totalSpent)

	return c.Send(msg, mainMenu())
}

// ─── Accounts ─────────────────────────────────────────────

func (b *Bot) handleAccounts(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	accs, err := b.DB.GetAccounts(ctx, userID)
	if err != nil {
		return c.Send("Error loading accounts", mainMenu())
	}

	if len(accs) == 0 {
		return c.Send("No accounts yet. Reply with:\n`+account Name type`\n\nTypes: `checking`, `savings`, `cash`, `credit_card`\n\nExample: `+account Monobank checking`", mainMenu())
	}

	var lines []string
	for _, a := range accs {
		emoji := "💳"
		switch a.Type {
		case "cash":
			emoji = "💵"
		case "savings":
			emoji = "🏦"
		case "credit_card":
			emoji = "💳"
		default:
			emoji = "🏛️"
		}
		lines = append(lines, fmt.Sprintf("%s *%s*: %.2f %s", emoji, a.Name, a.Balance, a.Currency))
	}

	return c.Send("💰 *Accounts*\n\n"+strings.Join(lines, "\n")+"\n\n_Add account:_ `+account Name type`", mainMenu())
}

// ─── Categories ───────────────────────────────────────────

func (b *Bot) handleCategories(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	cats, err := b.DB.GetCategories(ctx, userID)
	if err != nil {
		return c.Send("Error loading categories", mainMenu())
	}

	if len(cats) == 0 {
		return c.Send("No categories yet.\n\n_Add one:_ `+cat 🍞 Groceries`", mainMenu())
	}

	var lines []string
	for _, cat := range cats {
		lines = append(lines, fmt.Sprintf("%s %s", cat.Emoji, cat.Name))
	}

	return c.Send("🏷️ *Categories*\n\n"+strings.Join(lines, "\n")+"\n\n_Add:_ `+cat 🍞 Groceries`\n_Delete:_ `-cat Groceries`", mainMenu())
}

// ─── Budgets ──────────────────────────────────────────────

func (b *Bot) handleBudgets(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	cats, err := b.DB.GetCategories(ctx, userID)
	if err != nil || len(cats) == 0 {
		return c.Send("No categories yet. Create some first with *🏷️ Categories*.", mainMenu())
	}

	// Show current budgets + ability to set
	budgets, _ := b.DB.GetBudgets(ctx, userID)
	month := time.Now().Format("2006-01")

	budgetMap := make(map[int64]float64)
	for _, bd := range budgets {
		if bd.Month == month+"-01" {
			budgetMap[bd.CategoryID] = bd.Amount
		}
	}

	var lines []string
	for _, cat := range cats {
		amount := budgetMap[cat.ID]
		if amount > 0 {
			lines = append(lines, fmt.Sprintf("%s %s: *%.0f*", cat.Emoji, cat.Name, amount))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "_No budgets set for this month_")
	}

	msg := "🎯 *Monthly Budgets*\n\n" + strings.Join(lines, "\n")
	msg += "\n\n_Tap a category to set its budget:_"

	return c.Send(msg, budgetCategoryKeyboard(cats))
}

// ─── Recent ───────────────────────────────────────────────

func (b *Bot) handleRecent(c tele.Context) error {
	userID := c.Sender().ID
	ctx := context.Background()

	txs, err := b.DB.GetRecentTransactions(ctx, userID, 10)
	if err != nil {
		return c.Send("Error loading transactions", mainMenu())
	}

	if len(txs) == 0 {
		return c.Send("No transactions yet!", mainMenu())
	}

	var lines []string
	for _, t := range txs {
		sign := "➖"
		if t.Type == "income" {
			sign = "➕"
		} else if t.Type == "transfer" {
			sign = "↔️"
		}
		line := fmt.Sprintf("%s *%.2f* — %s", sign, t.Amount, t.AccountName)
		if t.CategoryEmoji != "" && t.CategoryName != "" {
			line += fmt.Sprintf(" | %s %s", t.CategoryEmoji, t.CategoryName)
		}
		line += fmt.Sprintf("\n  _%s_", t.CreatedAt.Format("Jan 2 15:04"))
		lines = append(lines, line)
	}

	return c.Send("📋 *Recent Transactions*\n\n"+strings.Join(lines, "\n"), mainMenu())
}

// ─── Cancel ───────────────────────────────────────────────

func (b *Bot) handleCancel(c tele.Context) error {
	userID := c.Sender().ID
	delete(b.States, userID)
	return c.Send("❌ Cancelled.", mainMenu())
}

// ─── Text handler (amount input, commands, etc.) ────────

func (b *Bot) handleText(c tele.Context) error {
	userID := c.Sender().ID
	text := c.Text()

	// Global commands (work during wizard too)
	switch {
	case strings.HasPrefix(text, "+account"):
		return b.createAccount(c)
	case strings.HasPrefix(text, "+cat"):
		return b.createCategory(c)
	case strings.HasPrefix(text, "-cat"):
		return b.deleteCategory(c)
	}

	state, inWizard := b.States[userID]
	if !inWizard {
		return c.Send("Tap a button or use `/menu`", mainMenu())
	}

	switch state.Step {
	case "awaiting_amount":
		return b.receiveAmount(c, state)
	case "awaiting_account":
		return b.receiveAccount(c, state)
	case "awaiting_income_amount":
		return b.receiveIncomeAmount(c, state)
	case "awaiting_income_account":
		return b.receiveIncomeAccount(c, state)
	case "awaiting_budget_amount":
		return b.receiveBudgetAmount(c, state)
	}

	return nil
}

// ─── Step: receive amount for expense ────────────────────

func (b *Bot) receiveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `42.50`", cancelBtn())
	}

	state.Amount = amount
	state.Step = "awaiting_account"

	userID := c.Sender().ID
	ctx := context.Background()
	accs, err := b.DB.GetAccounts(ctx, userID)
	if err != nil || len(accs) == 0 {
		return c.Send("No accounts! Create one with:\n`+account Name type`", mainMenu())
	}

	return c.Send(fmt.Sprintf("💰 *%.2f* — pick an account:", amount), accountKeyboard(accs))
}

// ─── Step: receive account for expense ────────────────────

func (b *Bot) receiveAccount(c tele.Context, state *userState) error {
	accountName := c.Text()
	userID := c.Sender().ID
	ctx := context.Background()

	accs, _ := b.DB.GetAccounts(ctx, userID)
	var acc *models.Account
	for _, a := range accs {
		if strings.EqualFold(a.Name, accountName) {
			acc = &a
			break
		}
	}
	if acc == nil {
		return c.Send(fmt.Sprintf("Account *%s* not found. Pick from the list or create one with `+account`.", accountName), mainMenu())
	}

	// Create transaction
	tx, err := b.DB.CreateTransaction(ctx, userID, acc.ID, &state.CategoryID, "expense", state.Amount, nil, state.Description)
	if err != nil {
		return c.Send("❌ Error saving transaction. Try again.", mainMenu())
	}

	delete(b.States, userID)

	catName := ""
	cats, _ := b.DB.GetCategories(ctx, userID)
	for _, c := range cats {
		if c.ID == state.CategoryID {
			catName = c.Name
			break
		}
	}

	msg := fmt.Sprintf("✅ Logged: *%.2f* on *%s* (%s)\n_%s_",
		tx.Amount, acc.Name, catName, tx.CreatedAt.Format("Jan 2 15:04"))
	return c.Send(msg, mainMenu())
}

// ─── Step: receive income amount ─────────────────────────

func (b *Bot) receiveIncomeAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `1500`", cancelBtn())
	}

	state.Amount = amount
	state.Step = "awaiting_income_account"

	userID := c.Sender().ID
	ctx := context.Background()
	accs, err := b.DB.GetAccounts(ctx, userID)
	if err != nil || len(accs) == 0 {
		return c.Send("No accounts! Create one with:\n`+account Name type`", mainMenu())
	}

	return c.Send(fmt.Sprintf("💰 +*%.2f* — pick an account:", amount), accountKeyboard(accs))
}

// ─── Step: receive account for income ────────────────────

func (b *Bot) receiveIncomeAccount(c tele.Context, state *userState) error {
	accountName := c.Text()
	userID := c.Sender().ID
	ctx := context.Background()

	accs, _ := b.DB.GetAccounts(ctx, userID)
	var acc *models.Account
	for _, a := range accs {
		if strings.EqualFold(a.Name, accountName) {
			acc = &a
			break
		}
	}
	if acc == nil {
		return c.Send("Account not found.", mainMenu())
	}

	tx, err := b.DB.CreateTransaction(ctx, userID, acc.ID, nil, "income", state.Amount, nil, "")
	if err != nil {
		return c.Send("❌ Error saving income. Try again.", mainMenu())
	}

	delete(b.States, userID)
	msg := fmt.Sprintf("✅ Income: +*%.2f* on *%s*\n_%s_",
		tx.Amount, acc.Name, tx.CreatedAt.Format("Jan 2 15:04"))
	return c.Send(msg, mainMenu())
}

// ─── Step: receive budget amount ─────────────────────────

func (b *Bot) receiveBudgetAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount < 0 {
		return c.Send("Please enter a valid number, e.g. `5000`", cancelBtn())
	}

	ctx := context.Background()
	month := time.Now().Format("2006-01") + "-01"

	_, err = b.DB.SetBudget(ctx, c.Sender().ID, state.EditingBudget, month, amount)
	if err != nil {
		return c.Send("❌ Error saving budget. Try again.", mainMenu())
	}

	delete(b.States, c.Sender().ID)

	// Get category name for confirmation
	cats, _ := b.DB.GetCategories(ctx, c.Sender().ID)
	catName := "category"
	for _, cat := range cats {
		if cat.ID == state.EditingBudget {
			catName = cat.Name
			break
		}
	}

	return c.Send(fmt.Sprintf("✅ Budget for *%s*: *%.0f* this month", catName, amount), mainMenu())
}

// ─── createAccount helper ─────────────────────────────────

func (b *Bot) createAccount(c tele.Context) error {
	parts := strings.Fields(c.Text())
	// +account Name type
	if len(parts) < 2 {
		return c.Send("Usage: `+account Name type`\nTypes: checking, savings, cash, credit_card", mainMenu())
	}

	name := parts[1]
	accType := "checking"
	if len(parts) >= 3 {
		accType = strings.ToLower(parts[2])
	}

	validTypes := map[string]bool{"checking": true, "savings": true, "cash": true, "credit_card": true}
	if !validTypes[accType] {
		return c.Send("Invalid type. Use: checking, savings, cash, credit_card", mainMenu())
	}

	ctx := context.Background()
	acc, err := b.DB.CreateAccount(ctx, c.Sender().ID, name, accType, "UAH", 0)
	if err != nil {
		return c.Send("❌ Error creating account. Does it already exist?", mainMenu())
	}

	emoji := "💳"
	if accType == "cash" {
		emoji = "💵"
	}
	return c.Send(fmt.Sprintf("✅ Account *%s* (%s) created! %s", acc.Name, acc.Type, emoji), mainMenu())
}

// ─── createCategory helper ────────────────────────────────

func (b *Bot) createCategory(c tele.Context) error {
	// +cat 🍞 Groceries
	text := strings.TrimPrefix(c.Text(), "+cat")
	text = strings.TrimSpace(text)

	parts := strings.Fields(text)
	if len(parts) < 1 {
		return c.Send("Usage: `+cat 🍞 Groceries`", mainMenu())
	}

	emoji := "📌"
	name := text

	// If first "word" looks like an emoji, extract it
	if len(parts) >= 2 {
		first := parts[0]
		if len([]rune(first)) <= 4 {
			emoji = first
			name = strings.Join(parts[1:], " ")
		}
	}

	ctx := context.Background()
	cat, err := b.DB.CreateCategory(ctx, c.Sender().ID, name, emoji)
	if err != nil {
		return c.Send("❌ Error creating category. Does it already exist?", mainMenu())
	}

	return c.Send(fmt.Sprintf("✅ Category %s *%s* created!", cat.Emoji, cat.Name), mainMenu())
}

// ─── deleteCategory helper ────────────────────────────────

func (b *Bot) deleteCategory(c tele.Context) error {
	name := strings.TrimPrefix(c.Text(), "-cat")
	name = strings.TrimSpace(name)

	if name == "" {
		return c.Send("Usage: `-cat Groceries`", mainMenu())
	}

	ctx := context.Background()
	cats, err := b.DB.GetCategories(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("❌ Error loading categories", mainMenu())
	}

	for _, cat := range cats {
		if strings.EqualFold(cat.Name, name) {
			if err := b.DB.DeleteCategory(ctx, cat.ID); err != nil {
				return c.Send("❌ Error deleting category", mainMenu())
			}
			return c.Send(fmt.Sprintf("✅ Deleted: %s %s", cat.Emoji, cat.Name), mainMenu())
		}
	}

	return c.Send(fmt.Sprintf("Category *%s* not found.", name), mainMenu())
}

// ─── Dynamic Callback Handler ─────────────────────────────
// Parses callback data prefixes: "cat_", "budget_", "acc_"

func (b *Bot) handleCallback(c tele.Context) error {
	data := c.Callback().Data
	userID := c.Sender().ID
	ctx := context.Background()

	switch {
	case strings.HasPrefix(data, "cat_"):
		// User picked a category for expense
		idStr := strings.TrimPrefix(data, "cat_")
		catID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
		}

		b.States[userID] = &userState{
			Step:       "awaiting_amount",
			CategoryID: catID,
			TxType:     "expense",
		}

		// Get category name for confirmation
		cats, _ := b.DB.GetCategories(ctx, userID)
		for _, cat := range cats {
			if cat.ID == catID {
				return c.Edit(fmt.Sprintf("%s *%s*\n\nEnter the amount:", cat.Emoji, cat.Name), cancelBtn())
			}
		}
		return c.Edit("Enter the amount:", cancelBtn())

	case strings.HasPrefix(data, "budget_"):
		// User picked a category to set budget for
		idStr := strings.TrimPrefix(data, "budget_")
		catID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
		}

		b.States[userID] = &userState{
			Step:          "awaiting_budget_amount",
			EditingBudget: catID,
		}

		cats, _ := b.DB.GetCategories(ctx, userID)
		for _, cat := range cats {
			if cat.ID == catID {
				return c.Edit(fmt.Sprintf("%s *%s*\n\nEnter the monthly budget amount:", cat.Emoji, cat.Name), cancelBtn())
			}
		}
		return c.Edit("Enter the monthly budget amount:", cancelBtn())

	case strings.HasPrefix(data, "acc_"):
		// User picked an account
		idStr := strings.TrimPrefix(data, "acc_")
		accID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid account"})
		}

		state, ok := b.States[userID]
		if !ok {
			return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
		}

		acc, err := b.DB.GetAccount(ctx, accID)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Account not found"})
		}

		switch state.TxType {
		case "expense":
			tx, err := b.DB.CreateTransaction(ctx, userID, acc.ID, &state.CategoryID, "expense", state.Amount, nil, state.Description)
			if err != nil {
				return c.Edit("❌ Error saving transaction. Try again.", mainMenu())
			}
			delete(b.States, userID)

			catName := ""
			cats, _ := b.DB.GetCategories(ctx, userID)
			for _, c := range cats {
				if c.ID == state.CategoryID {
					catName = c.Name
					break
				}
			}

			return c.Edit(fmt.Sprintf("✅ Logged: *%.2f* on *%s* (%s)\n_%s_",
				tx.Amount, acc.Name, catName, tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())

		case "income":
			tx, err := b.DB.CreateTransaction(ctx, userID, acc.ID, nil, "income", state.Amount, nil, "")
			if err != nil {
				return c.Edit("❌ Error saving income. Try again.", mainMenu())
			}
			delete(b.States, userID)

			return c.Edit(fmt.Sprintf("✅ Income: +*%.2f* on *%s*\n_%s_",
				tx.Amount, acc.Name, tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())
		}

		return c.Respond(&tele.CallbackResponse{Text: "Done!"})
	}

	return c.Respond()
}
