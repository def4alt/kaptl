package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
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

// dbTimeout is the maximum time for any single database operation.
const dbTimeout = 5 * time.Second

// maxNameLen is the maximum length for user-supplied names.
const maxNameLen = 100

// ─── Bot ──────────────────────────────────────────────────

// Bot carries runtime state for the telegram bot.
type Bot struct {
	Tele   *tele.Bot
	Store  models.Store
	mu     sync.Mutex
	States map[int64]*userState // telegramID -> wizard state
}

func New(store models.Store) (*Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN not set")
	}
	allowedID, err := strconv.ParseInt(os.Getenv("ALLOWED_TELEGRAM_ID"), 10, 64)
	if err != nil && os.Getenv("ALLOWED_TELEGRAM_ID") != "" {
		log.Printf("invalid ALLOWED_TELEGRAM_ID, auth disabled: %v", err)
		allowedID = 0
	}

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
		Store:  store,
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
			ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
			defer cancel()
			sender := c.Sender()
			_, err := b.Store.GetOrCreateUser(ctx, sender.ID, sender.Username, sender.FirstName, sender.LanguageCode)
			if err != nil {
				log.Printf("get or create user: %v", err)
			}
			return next(c)
		}
	})

	b.registerHandlers()
	b.registerCommands()
	return b, nil
}

// stateFor returns the wizard state for a user, safely.
func (b *Bot) stateFor(uid int64) *userState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.States[uid]
}

// setState stores wizard state for a user.
func (b *Bot) setState(uid int64, s *userState) {
	b.mu.Lock()
	b.States[uid] = s
	b.mu.Unlock()
}

// clearState removes wizard state for a user.
func (b *Bot) clearState(uid int64) {
	b.mu.Lock()
	delete(b.States, uid)
	b.mu.Unlock()
}

func (b *Bot) Start() {
	log.Println("🤖 Bot starting...")
	b.Tele.Start()
}

// ─── Register bot commands for Telegram autocomplete ─────

func (b *Bot) registerCommands() {
	cmds := []tele.Command{
		{Text: "start", Description: "show main menu"},
		{Text: "help", Description: "show help"},
		{Text: "cat", Description: "/cat add 🍞 Name | /cat rm Name | /cat list"},
		{Text: "acc", Description: "/acc add 💳 Name [currency] | /acc list"},
		{Text: "budget", Description: "/budget set CategoryName amount"},
	}
	if err := b.Tele.SetCommands(cmds); err != nil {
		log.Printf("register commands: %v", err)
	}
}

// ─── Handlers ─────────────────────────────────────────────

func (b *Bot) registerHandlers() {
	// Core commands
	b.Tele.Handle("/start", b.handleStart)
	b.Tele.Handle("/menu", b.handleStart)
	b.Tele.Handle("/help", b.handleHelp)

	// Slash commands for config
	b.Tele.Handle("/cat", b.handleCat)
	b.Tele.Handle("/acc", b.handleAcc)
	b.Tele.Handle("/budget", b.handleBudget)

	// Callback buttons (inline keyboard)
	b.Tele.Handle(&btnAddExpense, b.handleAddExpense)
	b.Tele.Handle(&btnAddIncome, b.handleAddIncome)
	b.Tele.Handle(&btnSummary, b.handleSummary)
	b.Tele.Handle(&btnAccounts, b.handleAccounts)
	b.Tele.Handle(&btnCategories, b.handleCategories)
	b.Tele.Handle(&btnBudgets, b.handleBudgetMenu)
	b.Tele.Handle(&btnRecent, b.handleRecent)
	b.Tele.Handle(&btnCancel, b.handleCancel)

	// Dynamic callbacks (category/account/budget picks)
	b.Tele.Handle(tele.OnCallback, b.handleCallback)

	// Text input (amount during wizard, or unrecognized)
	b.Tele.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleStart(c tele.Context) error {
	userID := c.Sender().ID
	b.clearState(userID)
	return c.Send("💰 *Kaptl* — expense tracker\n\nTrack your spending like a pro.", mainMenu())
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(`*Commands:*
/start – Main menu
/cat add 🍞 Name – Create category
/cat rm Name – Delete category
/cat list – List categories
/acc add 💳 Name [currency] – Create account
/acc list – List accounts
/budget set Name amount – Set monthly budget

*Quick expense:*
Tap "➕ Expense" → pick category → type amount → pick account → done!

*Currency defaults to EUR.*`, mainMenu())
}

// ─── /cat — manage categories ────────────────────────────

func (b *Bot) handleCat(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return b.handleCategories(c)
	}

	switch args[0] {
	case "add":
		return b.catAdd(c, args[1:])
	case "rm", "remove", "delete":
		return b.catRemove(c, args[1:])
	case "list", "ls":
		return b.handleCategories(c)
	default:
		return c.Send("Usage:\n`/cat add 🍞 Name`\n`/cat rm Name`\n`/cat list`", mainMenu())
	}
}

func (b *Bot) catAdd(c tele.Context, args []string) error {
	if len(args) < 1 {
		return c.Send("Usage: `/cat add 🍞 Name`", mainMenu())
	}

	emoji := "📌"
	rest := args

	// If first arg looks like a single emoji, extract it
	if len(args) >= 2 && len([]rune(args[0])) <= 4 {
		emoji = args[0]
		rest = args[1:]
	}
	name := strings.Join(rest, " ")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	cat, err := b.Store.CreateCategory(ctx, c.Sender().ID, name, emoji)
	if err != nil {
		return c.Send("❌ Category already exists or error occurred.", mainMenu())
	}
	return c.Send(fmt.Sprintf("✅ Created: %s *%s*", cat.Emoji, cat.Name), mainMenu())
}

func (b *Bot) catRemove(c tele.Context, args []string) error {
	if len(args) < 1 {
		return c.Send("Usage: `/cat rm Name`", mainMenu())
	}
	name := strings.Join(args, " ")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	for _, cat := range cats {
		if strings.EqualFold(cat.Name, name) {
			if err := b.Store.DeleteCategory(ctx, cat.ID); err != nil {
				return c.Send("❌ Error deleting category.", mainMenu())
			}
			return c.Send(fmt.Sprintf("✅ Deleted: %s %s", cat.Emoji, cat.Name), mainMenu())
		}
	}
	return c.Send(fmt.Sprintf("Category *%s* not found.", name), mainMenu())
}

// ─── /acc — manage accounts ───────────────────────────────

func (b *Bot) handleAcc(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return b.handleAccounts(c)
	}

	switch args[0] {
	case "add":
		return b.accAdd(c, args[1:])
	case "list", "ls":
		return b.handleAccounts(c)
	default:
		return c.Send("Usage:\n`/acc add 💳 Name [currency]`\n`/acc list`\n\nCurrency: EUR, USD, UAH, PLN... (default: EUR)", mainMenu())
	}
}

func (b *Bot) accAdd(c tele.Context, args []string) error {
	if len(args) < 1 {
		return c.Send("Usage: `/acc add 💳 Name [currency]`\nCurrency: EUR, USD, UAH, PLN... (default: EUR)", mainMenu())
	}

	emoji := "💳"
	name := strings.Join(args, " ")
	currency := "EUR"

	// If first arg looks like a single emoji, extract it
	first := []rune(args[0])
	if len(first) <= 4 && len(args) >= 2 && first[0] > 127 {
		emoji = args[0]
		rest := args[1:]
		if len(rest) == 0 {
			return c.Send("Usage: `/acc add 💳 Name [currency]`", mainMenu())
		}
		// Check if last arg is a 3-letter currency code
		if len(rest) >= 2 && len(rest[len(rest)-1]) == 3 {
			currency = strings.ToUpper(rest[len(rest)-1])
			name = strings.Join(rest[:len(rest)-1], " ")
		} else {
			name = strings.Join(rest, " ")
		}
	} else {
		// No emoji prefix — last arg might be currency
		if len(args) >= 2 && len(args[len(args)-1]) == 3 {
			currency = strings.ToUpper(args[len(args)-1])
			name = strings.Join(args[:len(args)-1], " ")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	acc, err := b.Store.CreateAccount(ctx, c.Sender().ID, name, emoji, currency, 0)
	if err != nil {
		return c.Send("❌ Error creating account. Does it already exist?", mainMenu())
	}

	return c.Send(fmt.Sprintf("✅ Created: %s *%s* (%s)", emoji, acc.Name, acc.Currency), mainMenu())
}

// ─── /budget — set budget ─────────────────────────────────

func (b *Bot) handleBudget(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 || args[0] != "set" {
		b.handleBudgetMenu(c)
		return nil
	}

	// /budget set Groceries 5000
	// Last arg is amount, everything before is category name
	if len(args) < 3 {
		return c.Send("Usage: `/budget set CategoryName amount`\nExample: `/budget set Groceries 5000`", mainMenu())
	}

	amount, err := strconv.ParseFloat(args[len(args)-1], 64)
	if err != nil || amount < 0 {
		return c.Send("Amount must be a number, e.g. `5000`", mainMenu())
	}

	catName := strings.Join(args[1:len(args)-1], " ")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	for _, cat := range cats {
		if strings.EqualFold(cat.Name, catName) {
			month := time.Now().Format("2006-01") + "-01"
			_, err := b.Store.SetBudget(ctx, c.Sender().ID, cat.ID, month, amount)
			if err != nil {
				return c.Send("❌ Error setting budget.", mainMenu())
			}
			return c.Send(fmt.Sprintf("✅ Budget for %s *%s*: *%.0f*/month", cat.Emoji, cat.Name, amount), mainMenu())
		}
	}
	return c.Send(fmt.Sprintf("Category *%s* not found.", catName), mainMenu())
}

// ─── Add Expense ──────────────────────────────────────────

func (b *Bot) handleAddExpense(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	cats, err := b.Store.GetCategories(ctx, userID)
	if err != nil || len(cats) == 0 {
		return c.Send("No categories yet! Use `/cat add 🍞 Name` to create one.", mainMenu())
	}

	return c.Send("*Pick a category:*", categoryKeyboard(cats))
}

// ─── Add Income ───────────────────────────────────────────

func (b *Bot) handleAddIncome(c tele.Context) error {
	userID := c.Sender().ID
	b.setState(userID, &userState{Step: "awaiting_income_amount", TxType: "income"})

	return c.Send("*Enter the income amount:*\n\nJust type a number, e.g. `1500`", cancelBtn())
}

// ─── Summary ──────────────────────────────────────────────

func (b *Bot) handleSummary(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	cats, err := b.Store.GetBudgetSummary(ctx, userID)
	if err != nil {
		log.Printf("get budget summary: %v", err)
		return c.Send("Error loading summary", mainMenu())
	}

	if len(cats) == 0 {
		return c.Send("No categories yet. Use `/cat add 🍞 Name` to create some.", mainMenu())
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
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	accs, err := b.Store.GetAccounts(ctx, userID)
	if err != nil {
		log.Printf("get accounts: %v", err)
		return c.Send("Error loading accounts", mainMenu())
	}

	if len(accs) == 0 {
		return c.Send("No accounts yet.\n\n`/acc add 💳 Name [currency]`", mainMenu())
	}

	var lines []string
	for _, a := range accs {
		lines = append(lines, fmt.Sprintf("%s *%s*: %.2f %s", a.Emoji, a.Name, a.Balance, a.Currency))
	}

	return c.Send("💰 *Accounts*\n\n"+strings.Join(lines, "\n")+"\n\n_Add:_ `/acc add 💳 Name [currency]`", mainMenu())
}

// ─── Categories (button handler) ──────────────────────────

func (b *Bot) handleCategories(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	cats, err := b.Store.GetCategories(ctx, userID)
	if err != nil {
		log.Printf("get categories: %v", err)
		return c.Send("Error loading categories", mainMenu())
	}

	if len(cats) == 0 {
		return c.Send("No categories yet.\n\n_Add:_ `/cat add 🍞 Name`", mainMenu())
	}

	var lines []string
	for _, cat := range cats {
		lines = append(lines, fmt.Sprintf("%s %s", cat.Emoji, cat.Name))
	}

	return c.Send("🏷️ *Categories*\n\n"+strings.Join(lines, "\n")+
		"\n\n_Add:_ `/cat add 🍞 Name`\n_Remove:_ `/cat rm Name`", mainMenu())
}

// ─── Budgets (button handler → shows picker) ──────────────

func (b *Bot) handleBudgetMenu(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	cats, err := b.Store.GetCategories(ctx, userID)
	if err != nil || len(cats) == 0 {
		return c.Send("No categories yet. Create some first with `/cat add`.", mainMenu())
	}

	budgets, _ := b.Store.GetBudgets(ctx, userID)
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
	msg += "\n\n_Tap a category to set its budget, or use_\n`/budget set Name amount`"

	return c.Send(msg, budgetCategoryKeyboard(cats))
}

// ─── Recent ───────────────────────────────────────────────

func (b *Bot) handleRecent(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	txs, err := b.Store.GetRecentTransactions(ctx, userID, 10)
	if err != nil {
		log.Printf("get recent transactions: %v", err)
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
	b.clearState(userID)
	return c.Send("❌ Cancelled.", mainMenu())
}

// ─── Text handler (amount input during wizard only) ──────

func (b *Bot) handleText(c tele.Context) error {
	userID := c.Sender().ID

	state, inWizard := b.stateFor(userID), b.stateFor(userID) != nil
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
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	accs, err := b.Store.GetAccounts(ctx, userID)
	if err != nil || len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	return c.Send(fmt.Sprintf("💰 *%.2f* — pick an account:", amount), accountKeyboard(accs))
}

// ─── Step: receive account for expense ────────────────────

func (b *Bot) receiveAccount(c tele.Context, state *userState) error {
	accountName := c.Text()
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	accs, _ := b.Store.GetAccounts(ctx, userID)
	var acc *models.Account
	for _, a := range accs {
		if strings.EqualFold(a.Name, accountName) {
			acc = &a
			break
		}
	}
	if acc == nil {
		return c.Send(fmt.Sprintf("Account *%s* not found. Use `/acc add` to create one.", accountName), mainMenu())
	}

	tx, err := b.Store.CreateTransaction(ctx, userID, acc.ID, &state.CategoryID, "expense", state.Amount, nil, state.Description)
	if err != nil {
		return c.Send("❌ Error saving transaction. Try again.", mainMenu())
	}

	b.clearState(userID)

	catName := ""
	cats, _ := b.Store.GetCategories(ctx, userID)
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
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	accs, err := b.Store.GetAccounts(ctx, userID)
	if err != nil || len(accs) == 0 {
		return c.Send("No accounts! Create one with `/acc add 💳 Name`", mainMenu())
	}

	return c.Send(fmt.Sprintf("💰 +*%.2f* — pick an account:", amount), accountKeyboard(accs))
}

// ─── Step: receive account for income ────────────────────

func (b *Bot) receiveIncomeAccount(c tele.Context, state *userState) error {
	accountName := c.Text()
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	accs, _ := b.Store.GetAccounts(ctx, userID)
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

	tx, err := b.Store.CreateTransaction(ctx, userID, acc.ID, nil, "income", state.Amount, nil, "")
	if err != nil {
		return c.Send("❌ Error saving income. Try again.", mainMenu())
	}

	b.clearState(userID)
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

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	month := time.Now().Format("2006-01") + "-01"

	_, err = b.Store.SetBudget(ctx, c.Sender().ID, state.EditingBudget, month, amount)
	if err != nil {
		return c.Send("❌ Error saving budget. Try again.", mainMenu())
	}

	b.clearState(c.Sender().ID)

	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	catName := "category"
	for _, cat := range cats {
		if cat.ID == state.EditingBudget {
			catName = cat.Name
			break
		}
	}

	return c.Send(fmt.Sprintf("✅ Budget for *%s*: *%.0f* this month", catName, amount), mainMenu())
}

// ─── Dynamic Callback Handler ─────────────────────────────
// All dynamic inline buttons (category picks, account picks, budget picks, cancel)
// route through here. Uses structured callback data: "prefix|id"

func (b *Bot) handleCallback(c tele.Context) error {
	data := c.Callback().Data
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	// Always acknowledge the callback so Telegram stops the spinner.
	// Specific handlers below override this with their own response.
	defer c.Respond()

	prefix, idStr := parseCallback(data)

	switch prefix {
	case cbCancel:
		b.clearState(userID)
		return c.Edit("❌ Cancelled.", mainMenu())

	case cbCat:
		catID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
		}

		b.setState(userID, &userState{
			Step:       "awaiting_amount",
			CategoryID: catID,
			TxType:     "expense",
		})

		cats, _ := b.Store.GetCategories(ctx, userID)
		for _, cat := range cats {
			if cat.ID == catID {
				return c.Edit(fmt.Sprintf("%s *%s*\n\nEnter the amount:", cat.Emoji, cat.Name), cancelBtn())
			}
		}
		return c.Edit("Enter the amount:", cancelBtn())

	case cbBudget:
		catID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
		}

		b.setState(userID, &userState{
			Step:          "awaiting_budget_amount",
			EditingBudget: catID,
		})

		cats, _ := b.Store.GetCategories(ctx, userID)
		for _, cat := range cats {
			if cat.ID == catID {
				return c.Edit(fmt.Sprintf("%s *%s*\n\nEnter the monthly budget amount:", cat.Emoji, cat.Name), cancelBtn())
			}
		}
		return c.Edit("Enter the monthly budget amount:", cancelBtn())

	case cbAcc:
		accID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Invalid account"})
		}

		state := b.stateFor(userID); ok := state != nil
		if !ok {
			return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
		}

		acc, err := b.Store.GetAccount(ctx, accID)
		if err != nil {
			return c.Respond(&tele.CallbackResponse{Text: "Account not found"})
		}

		switch state.TxType {
		case "expense":
			tx, err := b.Store.CreateTransaction(ctx, userID, acc.ID, &state.CategoryID, "expense", state.Amount, nil, state.Description)
			if err != nil {
				return c.Edit("❌ Error saving transaction. Try again.", mainMenu())
			}
			b.clearState(userID)

			catName := ""
			cats, _ := b.Store.GetCategories(ctx, userID)
			for _, c := range cats {
				if c.ID == state.CategoryID {
					catName = c.Name
					break
				}
			}

			return c.Edit(fmt.Sprintf("✅ Logged: *%.2f* on *%s* (%s)\n_%s_",
				tx.Amount, acc.Name, catName, tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())

		case "income":
			tx, err := b.Store.CreateTransaction(ctx, userID, acc.ID, nil, "income", state.Amount, nil, "")
			if err != nil {
				return c.Edit("❌ Error saving income. Try again.", mainMenu())
			}
			b.clearState(userID)

			return c.Edit(fmt.Sprintf("✅ Income: +*%.2f* on *%s*\n_%s_",
				tx.Amount, acc.Name, tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())
		}

		return c.Respond(&tele.CallbackResponse{Text: "Done!"})
	}

	return nil
}
