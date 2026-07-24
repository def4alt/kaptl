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
	Step          string // "awaiting_amount", "awaiting_account", "awaiting_move_target", etc.
	CategoryID    int64
	AccountID     int64  // source account for moves, or expense/income account
	TargetAccountID int64 // destination account for moves
	Amount        float64
	Description   string
	TxType        string // "expense", "income", or "transfer"
	EditingBudget int64  // category ID when setting budget
	TemplateMsgID int    // message ID of the progress template
	ChatID        int64  // chat where the template lives
	PrevStep      string // previous step for Back navigation
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
		{Text: "budget", Description: "/budget set Name amount [weekly|monthly|quarterly]"},
		{Text: "move", Description: "/move amount from Account to Account"},
		{Text: "group", Description: "/group add 📁 Name | /group rm Name | /group"},
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
	b.Tele.Handle("/move", b.handleMove)

	// Callback buttons (inline keyboard)
	b.Tele.Handle(&btnAddExpense, b.handleAddExpense)
	b.Tele.Handle(&btnAddIncome, b.handleAddIncome)
	b.Tele.Handle(&btnMove, b.handleMoveBtn)
	b.Tele.Handle(&btnSummary, b.handleSummary)
	b.Tele.Handle(&btnAccounts, b.handleAccounts)
	b.Tele.Handle(&btnCategories, b.handleCategories)
	b.Tele.Handle(&btnBudgets, b.handleBudgetMenu)
	b.Tele.Handle(&btnRecent, b.handleRecent)
	b.Tele.Handle(&btnCancel, b.handleCancel)

	// Group commands
	b.Tele.Handle("/group", b.handleGroup)

	// Dynamic callbacks (category/account/budget picks)
	// Registered with \f prefix — telebot routes menu.Data() callbacks
	// through static handler matching by Unique field.
	b.Tele.Handle("\f"+cbCat, b.handleCatPick)
	b.Tele.Handle("\f"+cbBudget, b.handleBudgetPick)
	b.Tele.Handle("\f"+cbAcc, b.handleAccPick)
	b.Tele.Handle("\f"+cbCancel, b.handleDynamicCancel)

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
/budget set Name amount [interval] – Set recurring budget
/group add 📁 Name – Create category group
/group rm Name – Delete group
/move amount from Account to Account – Transfer between accounts

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
	cat, err := b.Store.CreateCategory(ctx, c.Sender().ID, name, emoji, nil)
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

// ─── /group — manage category groups ──────────────────────

func (b *Bot) handleGroup(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
		groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)
		if len(groups) == 0 {
			return c.Send("No groups yet.\n\n`/group add 📁 Name` to create one.", mainMenu())
		}
		var lines []string
		for _, g := range groups {
			lines = append(lines, fmt.Sprintf("%s %s", g.Emoji, g.Name))
		}
		return c.Send("📁 *Groups*\n\n"+strings.Join(lines, "\n")+
			"\n\n_Add:_ `/group add 📁 Name`\n_Remove:_ `/group rm Name`", mainMenu())
	}

	switch args[0] {
	case "add":
		if len(args) < 2 {
			return c.Send("Usage: `/group add 📁 Name`", mainMenu())
		}
		emoji := "📁"
		rest := args[1:]
		first := []rune(args[1])
		if len(first) <= 4 && first[0] > 127 {
			emoji = args[1]
			rest = args[2:]
		}
		name := strings.Join(rest, " ")
		if name == "" {
			return c.Send("Usage: `/group add 📁 Name`", mainMenu())
		}

		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		g, err := b.Store.CreateGroup(ctx, c.Sender().ID, name, emoji)
		if err != nil {
			return c.Send("❌ Group already exists or error occurred.", mainMenu())
		}
		return c.Send(fmt.Sprintf("✅ Created group: %s *%s*", g.Emoji, g.Name), mainMenu())

	case "rm", "remove":
		if len(args) < 2 {
			return c.Send("Usage: `/group rm Name`", mainMenu())
		}
		name := strings.Join(args[1:], " ")
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)
		for _, g := range groups {
			if strings.EqualFold(g.Name, name) {
				if err := b.Store.DeleteGroup(ctx, g.ID); err != nil {
					return c.Send("❌ Error deleting group.", mainMenu())
				}
				return c.Send(fmt.Sprintf("✅ Deleted group: %s %s", g.Emoji, g.Name), mainMenu())
			}
		}
		return c.Send(fmt.Sprintf("Group *%s* not found.", name), mainMenu())

	default:
		return c.Send("Usage:\n`/group add 📁 Name`\n`/group rm Name`\n`/group` — list", mainMenu())
	}
}

// ─── /budget — set budget ─────────────────────────────────

func (b *Bot) handleBudget(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 || args[0] != "set" {
		b.handleBudgetMenu(c)
		return nil
	}

	// /budget set Groceries 5000 [interval]
	// interval: weekly, biweekly, monthly (default), quarterly, or Nd (e.g. 7d, 30d)
	if len(args) < 3 {
		return c.Send("Usage: `/budget set Name amount [interval]`\nExample: `/budget set Groceries 5000 monthly`\n\nIntervals: weekly, biweekly, monthly, quarterly, or 30d", mainMenu())
	}

	// Parse amount (last or second-to-last arg)
	amountIdx := len(args) - 1
	intervalDays, intervalMonths := defaultInterval()
	if len(args) >= 4 {
		intervalDays, intervalMonths = parseInterval(args[len(args)-1])
		if intervalDays == 0 && intervalMonths == 0 {
			// Last arg might be the amount, try second-to-last as interval
			intervalDays, intervalMonths = parseInterval(args[len(args)-1])
			if intervalDays == 0 && intervalMonths == 0 {
				amountIdx = len(args) - 1
			}
		} else {
			amountIdx = len(args) - 2
		}
	}

	amount, err := strconv.ParseFloat(args[amountIdx], 64)
	if err != nil || amount < 0 {
		return c.Send("Amount must be a number, e.g. `5000`", mainMenu())
	}

	catName := strings.Join(args[1:amountIdx], " ")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	for _, cat := range cats {
		if strings.EqualFold(cat.Name, catName) {
			bd, err := b.Store.SetBudget(ctx, c.Sender().ID, cat.ID, intervalDays, intervalMonths, amount)
			if err != nil {
				return c.Send("❌ Error setting budget.", mainMenu())
			}
			return c.Send(fmt.Sprintf("✅ Budget for %s *%s*: *%.0f* (%s)\n_Next reset: %s_",
				cat.Emoji, cat.Name, amount, bd.Description(),
				bd.PeriodStart.AddDate(0, bd.IntervalMonths, bd.IntervalDays).Format("Jan 2")), mainMenu())
		}
	}
	return c.Send(fmt.Sprintf("Category *%s* not found.", catName), mainMenu())
}

func defaultInterval() (int, int) { return 0, 1 } // monthly
func parseInterval(s string) (int, int) {
	switch strings.ToLower(s) {
	case "weekly":
		return 7, 0
	case "biweekly":
		return 14, 0
	case "monthly":
		return 0, 1
	case "quarterly":
		return 0, 3
	default:
		// Try Nd format: "7d", "30d"
		if strings.HasSuffix(s, "d") {
			if d, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && d > 0 {
				return d, 0
			}
		}
		return 0, 0
	}
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
	b.setState(userID, &userState{
		Step:       "awaiting_income_amount",
		TxType:     "income",
		TemplateMsgID: c.Message().ID,
		ChatID:     c.Chat().ID,
	})

	text := progressTemplate("➕ *New Income*", map[string]string{
		"Amount":  "—",
		"Account": "—",
	})
	return c.Edit(text, cancelBtn())
}

// ─── Move / Transfer between accounts ──────────────────────

// handleMove is the /move slash command: /move 500 from Mono to Cash
func (b *Bot) handleMove(c tele.Context) error {
	userID := c.Sender().ID
	text := strings.TrimSpace(c.Message().Payload)

	// Try parsing: "500 from Mono to Cash"
	parts := strings.Fields(text)
	if len(parts) < 5 || strings.ToLower(parts[1]) != "from" || strings.ToLower(parts[3]) != "to" {
		return c.Send("Usage: `/move <amount> from <Account> to <Account>`\n\nExample: `/move 500 from Mono to Cash`\n\nOr tap *🔀 Move* for interactive mode.", mainMenu())
	}

	amount, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || amount <= 0 {
		return c.Send("❌ Invalid amount. Use a positive number, e.g. `500`", mainMenu())
	}

	fromName := parts[2]
	toName := parts[4]

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	accs, err := b.Store.GetAccounts(ctx, userID)
	if err != nil {
		log.Printf("get accounts for move: %v", err)
		return c.Send("Error loading accounts", mainMenu())
	}

	var from, to *models.Account
	for i, a := range accs {
		if strings.EqualFold(a.Name, fromName) {
			from = &accs[i]
		}
		if strings.EqualFold(a.Name, toName) {
			to = &accs[i]
		}
	}

	if from == nil {
		return c.Send(fmt.Sprintf("❌ Account *%s* not found.", fromName), mainMenu())
	}
	if to == nil {
		return c.Send(fmt.Sprintf("❌ Account *%s* not found.", toName), mainMenu())
	}
	if from.ID == to.ID {
		return c.Send("❌ Source and destination must be different accounts.", mainMenu())
	}

	tx, err := b.Store.CreateTransaction(ctx, userID, from.ID, nil, "transfer", amount, &to.ID, fmt.Sprintf("→ %s", to.Name))
	if err != nil {
		log.Printf("create transfer: %v", err)
		return c.Send("❌ Error creating transfer.", mainMenu())
	}

	return c.Send(fmt.Sprintf("✅ Transferred *%.2f* %s\n%s %s → %s %s\n_%s_",
		amount, from.Currency,
		from.Emoji, from.Name,
		to.Emoji, to.Name,
		tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())
}

// handleMoveBtn starts the interactive move wizard.
func (b *Bot) handleMoveBtn(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	accs, err := b.Store.GetAccounts(ctx, userID)
	if err != nil {
		log.Printf("get accounts for move btn: %v", err)
		return c.Send("Error loading accounts", mainMenu())
	}
	if len(accs) < 2 {
		return c.Send("Need at least 2 accounts to transfer. Use `/acc add 💳 Name` first.", mainMenu())
	}

	b.setState(userID, &userState{
		Step:       "awaiting_move_source",
		TxType:     "transfer",
		TemplateMsgID: c.Message().ID,
		ChatID:     c.Chat().ID,
	})

	text := progressTemplate("🔀 *Transfer*", map[string]string{
		"From":   "—",
		"To":     "—",
		"Amount": "—",
	})
	return c.Edit(text, accountKeyboard(accs))
}

// ─── Summary ──────────────────────────────────────────────

func (b *Bot) handleSummary(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	rows, err := b.Store.GetBudgetSummary(ctx, userID, 0)
	if err != nil {
		log.Printf("get budget summary: %v", err)
		return c.Send("Error loading summary", mainMenu())
	}

	if len(rows) == 0 {
		return c.Send("No categories yet. Use `/cat add 🍞 Name` to create some.", mainMenu())
	}

	totalBudget := 0.0
	totalSpent := 0.0

	var lines []string
	lastGroup := ""

	for _, r := range rows {
		if r.GroupName != "" && r.GroupName != lastGroup {
			lastGroup = r.GroupName
			lines = append(lines, fmt.Sprintf("\n🏷️ *%s*", lastGroup))
		}
		line := fmt.Sprintf("%s *%s*: %.0f / %.0f (%.0f left)",
			r.Emoji, r.Name, r.Spent, r.Available, r.Remaining)
		if r.Rollover > 0 {
			line += fmt.Sprintf("  _+%.0f rollover_", r.Rollover)
		}
		lines = append(lines, line)
		totalBudget += r.Available
		totalSpent += r.Spent
	}

	msg := fmt.Sprintf("📊 *Budget Summary — %s*\n", time.Now().Format("January 2006"))
	if rta, err := b.Store.GetReadyToAssign(ctx, userID); err == nil {
		color := "🟢"
		if rta < 0 {
			color = "🔴"
		}
		msg += fmt.Sprintf("\n💵 *Ready to Assign:* %s €%.0f\n", color, rta)
	}
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

	budgetMap := make(map[int64]*models.Budget)
	for i, bd := range budgets {
		budgetMap[bd.CategoryID] = &budgets[i]
	}

	var lines []string
	for _, cat := range cats {
		if bd, ok := budgetMap[cat.ID]; ok {
			lines = append(lines, fmt.Sprintf("%s %s: *%.0f* (%s)",
				cat.Emoji, cat.Name, bd.Amount, bd.Description()))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "_No budgets set_")
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
		if t.Type == "transfer" && t.Description != "" {
			line += " " + t.Description
		}
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
	case "awaiting_move_amount":
		return b.receiveMoveAmount(c, state)
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

	// Look up category name
	cats, _ := b.Store.GetCategories(ctx, userID)
	catName := "—"
	for _, cat := range cats {
		if cat.ID == state.CategoryID {
			catName = cat.Emoji + " " + cat.Name
			break
		}
	}

	fields := map[string]string{
		"Category": catName,
		"Amount":   fmt.Sprintf("€%.2f", amount),
		"Account":  "—",
	}
	text := progressTemplate("➖ *New Expense*", fields)

	if state.TemplateMsgID != 0 {
		b.editTemplate(state, text, accountKeyboard(accs))
	}
	return c.Send("\u200b", &tele.SendOptions{ReplyMarkup: &tele.ReplyMarkup{}}) // dummy to clear input
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

	text := progressTemplate("➕ *New Income*", map[string]string{
		"Amount":  fmt.Sprintf("€%.2f", amount),
		"Account": "—",
	})

	if state.TemplateMsgID != 0 {
		b.editTemplate(state, text, accountKeyboard(accs))
	}
	return c.Send("\u200b") // clear input
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

// ─── Step: receive move amount ────────────────────────────

func (b *Bot) receiveMoveAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount <= 0 {
		return c.Send("Please enter a valid number, e.g. `500`", cancelBtn())
	}

	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	srcAcc, _ := b.Store.GetAccount(ctx, state.AccountID)
	dstAcc, _ := b.Store.GetAccount(ctx, state.TargetAccountID)
	if srcAcc == nil || dstAcc == nil {
		b.clearState(userID)
		return c.Send("❌ Account not found. Start over.", mainMenu())
	}

	_, err = b.Store.CreateTransaction(ctx, userID, state.AccountID, nil, "transfer", amount, &state.TargetAccountID, fmt.Sprintf("→ %s", dstAcc.Name))
	if err != nil {
		log.Printf("create transfer: %v", err)
		b.clearState(userID)
		return c.Send("❌ Error creating transfer.", mainMenu())
	}

	b.clearState(userID)

	text := progressTemplate("🔀 *Transfer*", map[string]string{
		"From":   srcAcc.Emoji + " " + srcAcc.Name,
		"To":     dstAcc.Emoji + " " + dstAcc.Name,
		"Amount": fmt.Sprintf("€%.2f", amount),
	})
	return c.Send("✅ Transferred!\n\n"+text, mainMenu())
}

// ─── Step: receive budget amount ─────────────────────────

func (b *Bot) receiveBudgetAmount(c tele.Context, state *userState) error {
	amount, err := strconv.ParseFloat(strings.TrimSpace(c.Text()), 64)
	if err != nil || amount < 0 {
		return c.Send("Please enter a valid number, e.g. `5000`", cancelBtn())
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()
	intervalDays, intervalMonths := defaultInterval()

	_, err = b.Store.SetBudget(ctx, c.Sender().ID, state.EditingBudget, intervalDays, intervalMonths, amount)
	if err != nil {
		log.Printf("set budget: %v", err)
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

	return c.Send(fmt.Sprintf("✅ Budget for *%s*: *%.0f* (monthly)", catName, amount), mainMenu())
}

// ─── Dynamic callback handlers (registered per prefix) ─────
// telebot routes menu.Data(prefix, id) callbacks through \f+prefix.
// c.Callback().Unique = prefix, c.Callback().Data = payload.

func (b *Bot) handleCatPick(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	defer c.Respond()

	idStr := c.Callback().Data
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
	}

	// Look up category name
	cats, _ := b.Store.GetCategories(ctx, userID)
	var catName string
	for _, cat := range cats {
		if cat.ID == catID {
			catName = cat.Emoji + " " + cat.Name
			break
		}
	}

	b.setState(userID, &userState{
		Step:       "awaiting_amount",
		CategoryID: catID,
		TxType:     "expense",
		TemplateMsgID: c.Message().ID,
		ChatID:     c.Chat().ID,
		PrevStep:   "pick_category",
	})

	fields := map[string]string{
		"Category": catName,
		"Amount":   "—",
		"Account":  "—",
	}
	text := progressTemplate("➖ *New Expense*", fields)
	return c.Edit(text, cancelBtn())
}

func (b *Bot) handleBudgetPick(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	defer c.Respond()

	idStr := c.Callback().Data
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Printf("budget pick parse: %v", err)
		return c.Respond(&tele.CallbackResponse{Text: "Invalid category"})
	}

	b.setState(userID, &userState{
		Step:          "awaiting_budget_amount",
		EditingBudget: catID,
	})

	log.Printf("budget pick: category=%d, user=%d", catID, userID)

	cats, _ := b.Store.GetCategories(ctx, userID)
	for _, cat := range cats {
		if cat.ID == catID {
			err := c.Edit(fmt.Sprintf("%s *%s*\n\nEnter the monthly budget amount:", cat.Emoji, cat.Name), cancelBtn())
			if err != nil {
				log.Printf("budget pick edit: %v", err)
			}
			return err
		}
	}
	return c.Edit("Enter the monthly budget amount:", cancelBtn())
}

func (b *Bot) handleAccPick(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout); defer cancel()

	defer c.Respond()

	idStr := c.Callback().Data
	accID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Invalid account"})
	}

	state := b.stateFor(userID)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	acc, err := b.Store.GetAccount(ctx, accID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Account not found"})
	}

	switch state.TxType {
	case "expense":
		_, err := b.Store.CreateTransaction(ctx, userID, acc.ID, &state.CategoryID, "expense", state.Amount, nil, state.Description)
		if err != nil {
			return c.Edit("❌ Error saving transaction. Try again.", mainMenu())
		}
		b.clearState(userID)

		catName := ""
		cats, _ := b.Store.GetCategories(ctx, userID)
		for _, c := range cats {
			if c.ID == state.CategoryID {
				catName = c.Emoji + " " + c.Name
				break
			}
		}

		text := progressTemplate("➖ *New Expense*", map[string]string{
			"Category": catName,
			"Amount":   fmt.Sprintf("€%.2f", state.Amount),
			"Account":  acc.Emoji + " " + acc.Name,
		})
		return c.Edit("✅ Logged!\n\n" + text, mainMenu())

	case "income":
		_, err := b.Store.CreateTransaction(ctx, userID, acc.ID, nil, "income", state.Amount, nil, "")
		if err != nil {
			return c.Edit("❌ Error saving income. Try again.", mainMenu())
		}
		b.clearState(userID)

		text := progressTemplate("➕ *New Income*", map[string]string{
			"Amount":  fmt.Sprintf("€%.2f", state.Amount),
			"Account": acc.Emoji + " " + acc.Name,
		})
		return c.Edit("✅ Income logged!\n\n" + text, mainMenu())

	case "transfer":
		switch state.Step {
		case "awaiting_move_source":
			state.Step = "awaiting_move_target"
			state.AccountID = acc.ID
			accs, _ := b.Store.GetAccounts(ctx, userID)
			text := progressTemplate("🔀 *Transfer*", map[string]string{
				"From":   acc.Emoji + " " + acc.Name,
				"To":     "—",
				"Amount": "—",
			})
			return c.Edit(text, accountKeyboardExclude(accs, acc.ID))

		case "awaiting_move_target":
			state.Step = "awaiting_move_amount"
			state.TargetAccountID = acc.ID
			srcAcc, _ := b.Store.GetAccount(ctx, state.AccountID)
			srcName := "Unknown"
			if srcAcc != nil {
				srcName = srcAcc.Emoji + " " + srcAcc.Name
			}
			text := progressTemplate("🔀 *Transfer*", map[string]string{
				"From":   srcName,
				"To":     acc.Emoji + " " + acc.Name,
				"Amount": "—",
			})
			return c.Edit(text, cancelBtn())
		}
	}

	return c.Respond(&tele.CallbackResponse{Text: "Done!"})
}

func (b *Bot) handleDynamicCancel(c tele.Context) error {
	userID := c.Sender().ID
	defer c.Respond()
	b.clearState(userID)
	return c.Edit("❌ Cancelled.", mainMenu())
}
