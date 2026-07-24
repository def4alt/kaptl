package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── /cat ──────────────────────────────────────────────────

func (b *Bot) handleCat(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		return listView(c, b.Store, ctx, c.Sender().ID)
	}

	switch args[0] {
	case "add":
		return b.catAdd(c, args[1:])
	case "rm", "remove", "delete":
		return b.catRemove(c, args[1:])
	case "list", "ls":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		return listView(c, b.Store, ctx, c.Sender().ID)
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
	if len(args) >= 2 && len([]rune(args[0])) <= 4 {
		emoji = args[0]
		rest = args[1:]
	}
	name := strings.Join(rest, " ")

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
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

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
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

// ─── /acc ──────────────────────────────────────────────────

func (b *Bot) handleAcc(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
		return c.Send(msgAccounts(accs), mainMenu())
	}
	switch args[0] {
	case "add":
		return b.accAdd(c, args[1:])
	case "list", "ls":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)
		return c.Send(msgAccounts(accs), mainMenu())
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

	first := []rune(args[0])
	if len(first) <= 4 && len(args) >= 2 && first[0] > 127 {
		emoji = args[0]
		rest := args[1:]
		if len(rest) == 0 {
			return c.Send("Usage: `/acc add 💳 Name [currency]`", mainMenu())
		}
		if len(rest) >= 2 && len(rest[len(rest)-1]) == 3 {
			currency = strings.ToUpper(rest[len(rest)-1])
			name = strings.Join(rest[:len(rest)-1], " ")
		} else {
			name = strings.Join(rest, " ")
		}
	} else if len(args) >= 2 && len(args[len(args)-1]) == 3 {
		currency = strings.ToUpper(args[len(args)-1])
		name = strings.Join(args[:len(args)-1], " ")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	acc, err := b.Store.CreateAccount(ctx, c.Sender().ID, name, emoji, currency, 0)
	if err != nil {
		return c.Send("❌ Error creating account. Does it already exist?", mainMenu())
	}
	return c.Send(fmt.Sprintf("✅ Created: %s *%s* (%s)", emoji, acc.Name, acc.Currency), mainMenu())
}

// ─── /budget ───────────────────────────────────────────────

func (b *Bot) handleBudget(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 || args[0] != "set" {
		return b.handleBudgetMenu(c)
	}
	if len(args) < 3 {
		return c.Send("Usage: `/budget set Name amount [interval]`\nExample: `/budget set Groceries 5000 monthly`\n\nIntervals: weekly, biweekly, monthly, quarterly, or 30d", mainMenu())
	}

	amountIdx := len(args) - 1
	intervalDays, intervalMonths := defaultInterval()
	if len(args) >= 4 {
		d, m := parseInterval(args[len(args)-1])
		if d != 0 || m != 0 {
			amountIdx = len(args) - 2
			intervalDays, intervalMonths = d, m
		}
	}

	amount, err := strconv.ParseFloat(args[amountIdx], 64)
	if err != nil || amount < 0 {
		return c.Send("Amount must be a number, e.g. `5000`", mainMenu())
	}

	catName := strings.Join(args[1:amountIdx], " ")
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

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

// ─── /move ─────────────────────────────────────────────────

func (b *Bot) handleMove(c tele.Context) error {
	text := strings.TrimSpace(c.Message().Payload)
	parts := strings.Fields(text)
	if len(parts) < 5 || strings.ToLower(parts[1]) != "from" || strings.ToLower(parts[3]) != "to" {
		return c.Send("Usage: `/move <amount> from <Account> to <Account>`\n\nExample: `/move 500 from Mono to Cash`\n\nOr tap *🔀 Move* for interactive mode.", mainMenu())
	}

	amount, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || amount <= 0 {
		return c.Send("❌ Invalid amount. Use a positive number, e.g. `500`", mainMenu())
	}

	fromName, toName := parts[2], parts[4]
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	accs, err := b.Store.GetAccounts(ctx, c.Sender().ID)
	if err != nil {
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

	tx, err := b.Store.CreateTransaction(ctx, c.Sender().ID, from.ID, nil, "transfer", amount, &to.ID, fmt.Sprintf("→ %s", to.Name))
	if err != nil {
		return c.Send("❌ Error creating transfer.", mainMenu())
	}

	return c.Send(fmt.Sprintf("✅ Transferred *%.2f* %s\n%s %s → %s %s\n_%s_",
		amount, from.Currency, from.Emoji, from.Name, to.Emoji, to.Name,
		tx.CreatedAt.Format("Jan 2 15:04")), mainMenu())
}

// ─── /group ────────────────────────────────────────────────

func (b *Bot) handleGroup(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)
		return c.Send(msgGroups(groups), mainMenu())
	}
	switch args[0] {
	case "add":
		return b.groupAdd(c, args[1:])
	case "rm", "remove":
		return b.groupRemove(c, args[1:])
	default:
		return c.Send("Usage:\n`/group add 📁 Name`\n`/group rm Name`\n`/group` — list", mainMenu())
	}
}

func (b *Bot) groupAdd(c tele.Context, args []string) error {
	if len(args) < 1 {
		return c.Send("Usage: `/group add 📁 Name`", mainMenu())
	}
	emoji := "📁"
	rest := args
	if len([]rune(args[0])) <= 4 && args[0][0] > 127 && len(args) >= 2 {
		emoji = args[0]
		rest = args[1:]
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
}

func (b *Bot) groupRemove(c tele.Context, args []string) error {
	if len(args) < 1 {
		return c.Send("Usage: `/group rm Name`", mainMenu())
	}
	name := strings.Join(args, " ")
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
}

// ─── Shared view helper ────────────────────────────────────

func listView(c tele.Context, store models.Store, ctx context.Context, userID int64) error {
	cats, _ := store.GetCategories(ctx, userID)
	groups, _ := store.GetGroups(ctx, userID)
	return c.Send(msgCategories(cats, groups), mainMenu())
}

// ─── Interval parsing ─────────────────────────────────────

func defaultInterval() (int, int) { return 0, 1 }
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
		if strings.HasSuffix(s, "d") {
			if d, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && d > 0 {
				return d, 0
			}
		}
		return 0, 0
	}
}

// unused import guard
