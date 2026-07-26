package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/bot/view"
	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── /cat ──────────────────────────────────────────────────

func (b *Bot) handleCat(c tele.Context) error {
	h := b.withCtx(c)
	defer h.done()
	args := c.Args()
	if len(args) == 0 {
		return h.send(view.Categories(h.cats(), h.groups()))
	}
	switch args[0] {
	case "add":
		return b.catAdd(h, args[1:])
	case "rm", "remove", "delete":
		return b.catRemove(h, args[1:])
	case "list", "ls":
		return h.send(view.Categories(h.cats(), h.groups()))
	default:
		return h.send("Usage:\n`/cat add 🍞 Name`\n`/cat rm Name`\n`/cat list`")
	}
}

func (b *Bot) catAdd(h *hctx, args []string) error {
	if len(args) < 1 {
		return h.send("Usage: `/cat add 🍞 Name`")
	}
	emoji, rest := parseEmojiArgs(args, "📌")
	name := strings.Join(rest, " ")

	cat, err := h.Bot.Store.CreateCategory(h.DB, h.UID, name, emoji, nil)
	if err != nil {
		return h.send(view.Error("Category already exists or error occurred."))
	}
	return h.send(view.Created(cat.Emoji, cat.Name, ""))
}

func (b *Bot) catRemove(h *hctx, args []string) error {
	if len(args) < 1 {
		return h.send("Usage: `/cat rm Name`")
	}
	name := strings.Join(args, " ")
	cats := h.cats()

	var catEmoji string
	deleted := false
	for _, c := range cats {
		if strings.EqualFold(c.Name, name) {
			catEmoji = c.Emoji
			if err := h.Bot.Store.DeleteCategory(h.DB, h.UID, c.ID); err != nil {
				return h.send(view.Error("Error deleting category."))
			}
			deleted = true
			break
		}
	}
	if !deleted {
		return h.send(fmt.Sprintf("Category *%s* not found.", name))
	}
	return h.send(view.Deleted(catEmoji, name, "category"))
}

// ─── /acc ──────────────────────────────────────────────────

func (b *Bot) handleAcc(c tele.Context) error {
	h := b.withCtx(c)
	defer h.done()
	args := c.Args()
	if len(args) == 0 {
		return h.send(view.Accounts(h.accs()))
	}
	switch args[0] {
	case "add":
		return b.accAdd(h, args[1:])
	case "list", "ls":
		return h.send(view.Accounts(h.accs()))
	default:
		return h.send("Usage:\n`/acc add 💳 Name [currency]`\n`/acc list`\n\nCurrency: EUR, USD, UAH, PLN... (default: EUR)")
	}
}

func (b *Bot) accAdd(h *hctx, args []string) error {
	if len(args) < 1 {
		return h.send("Usage: `/acc add 💳 Name [currency]`\nCurrency: EUR, USD, UAH, PLN... (default: EUR)")
	}
	emoji := "💳"
	name := strings.Join(args, " ")
	currency := "EUR"

	first := []rune(args[0])
	if len(first) <= 4 && len(args) >= 2 && first[0] > 127 {
		emoji = args[0]
		rest := args[1:]
		if len(rest) == 0 {
			return h.send("Usage: `/acc add 💳 Name [currency]`")
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
	currency, err := models.NormalizeCurrency(currency)
	if err != nil {
		return h.send("Unsupported currency. Choose EUR, USD, UAH, PLN, or GBP.")
	}

	acc, err := h.Bot.Store.CreateAccount(h.DB, h.UID, name, emoji, currency, 0)
	if err != nil {
		return h.send(view.Error("Error creating account. Does it already exist?"))
	}
	return h.send(fmt.Sprintf("✅ Created: %s *%s* (%s)", emoji, acc.Name, acc.Currency))
}

// ─── /budget ───────────────────────────────────────────────

func (b *Bot) handleBudget(c tele.Context) error {
	h := b.withCtx(c)
	defer h.done()
	args := c.Args()
	if len(args) < 2 || args[0] != "set" {
		return b.handleBudgetMenu(c)
	}
	if len(args) < 3 {
		return h.send("Usage: `/budget set Name amount [currency] [interval]`\nExample: `/budget set Groceries 5000 UAH monthly`\n\nIntervals: weekly, biweekly, monthly, quarterly, or 30d")
	}

	end := len(args)
	intervalDays, intervalMonths := defaultInterval()
	if d, m := parseInterval(args[end-1]); d != 0 || m != 0 {
		intervalDays, intervalMonths = d, m
		end--
	}

	currency := "EUR"
	if end >= 4 && len(args[end-1]) == 3 {
		var err error
		currency, err = models.NormalizeCurrency(args[end-1])
		if err != nil {
			return h.send("Unsupported currency. Choose EUR, USD, UAH, PLN, or GBP.")
		}
		end--
	}
	amountIdx := end - 1
	amount, err := strconv.ParseFloat(args[amountIdx], 64)
	if err != nil || amount < 0 {
		return h.send("Amount must be a number, e.g. `5000`")
	}

	catName := strings.Join(args[1:amountIdx], " ")
	for _, cat := range h.cats() {
		if strings.EqualFold(cat.Name, catName) {
			bd, err := h.Bot.Store.SetBudget(h.DB, h.UID, cat.ID, currency, intervalDays, intervalMonths, amount)
			if err != nil {
				return h.send(view.Error("Error setting budget."))
			}
			return h.send(fmt.Sprintf("✅ Budget for %s *%s*: *%s* (%s)",
				cat.Emoji, cat.Name, view.FormatMoney(amount, currency, 0), bd.Description()))
		}
	}
	return h.send(fmt.Sprintf("Category *%s* not found.", catName))
}

// ─── /move ─────────────────────────────────────────────────

func (b *Bot) handleMove(c tele.Context) error {
	h := b.withCtx(c)
	defer h.done()
	text := strings.TrimSpace(c.Message().Payload)
	parts := strings.Fields(text)
	if len(parts) < 5 || strings.ToLower(parts[1]) != "from" || strings.ToLower(parts[3]) != "to" {
		return h.send("Usage: `/move <amount> from <Account> to <Account>`\n\nExample: `/move 500 from Mono to Cash`\n\nOr tap *🔀 Move* for interactive mode.")
	}

	amount, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || amount <= 0 {
		return h.send("❌ Invalid amount. Use a positive number, e.g. `500`")
	}

	fromName, toName := parts[2], parts[4]
	accs := h.accs()

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
		return h.send(fmt.Sprintf("❌ Account *%s* not found.", fromName))
	}
	if to == nil {
		return h.send(fmt.Sprintf("❌ Account *%s* not found.", toName))
	}
	if from.ID == to.ID {
		return h.send("❌ Source and destination must be different accounts.")
	}
	if from.Currency != to.Currency {
		return h.send("❌ Accounts use different currencies. Cross-currency transfers need an explicit exchange rate and destination amount.")
	}

	tx, err := h.Bot.Store.CreateTransaction(h.DB, h.UID, from.ID, nil, "transfer", amount, &to.ID, fmt.Sprintf("→ %s", to.Name))
	if err != nil {
		return h.send(view.Error("Error creating transfer."))
	}

	return h.send(fmt.Sprintf("✅ Transferred *%.2f* %s\n%s %s → %s %s\n_%s_",
		amount, from.Currency, from.Emoji, from.Name, to.Emoji, to.Name,
		tx.CreatedAt.Format("Jan 2 15:04")))
}

// ─── /group ────────────────────────────────────────────────

func (b *Bot) handleGroup(c tele.Context) error {
	h := b.withCtx(c)
	defer h.done()
	args := c.Args()
	if len(args) == 0 {
		return h.send(view.Groups(h.groups()))
	}
	switch args[0] {
	case "add":
		return b.groupAdd(h, args[1:])
	case "rm", "remove":
		return b.groupRemove(h, args[1:])
	default:
		return h.send("Usage:\n`/group add 📁 Name`\n`/group rm Name`\n`/group` — list")
	}
}

func (b *Bot) groupAdd(h *hctx, args []string) error {
	if len(args) < 1 {
		return h.send("Usage: `/group add 📁 Name`")
	}
	emoji, rest := parseEmojiArgs(args, "📁")
	name := strings.Join(rest, " ")
	if name == "" {
		return h.send("Usage: `/group add 📁 Name`")
	}
	g, err := h.Bot.Store.CreateGroup(h.DB, h.UID, name, emoji)
	if err != nil {
		return h.send(view.Error("Group already exists or error occurred."))
	}
	return h.send(view.Created(g.Emoji, g.Name, "group"))
}

func (b *Bot) groupRemove(h *hctx, args []string) error {
	if len(args) < 1 {
		return h.send("Usage: `/group rm Name`")
	}
	name := strings.Join(args, " ")
	groups := h.groups()

	found, err := deleteByName(groups, name, func(g models.CategoryGroup) string { return g.Name },
		func(g models.CategoryGroup) error { return h.Bot.Store.DeleteGroup(h.DB, h.UID, g.ID) },
	)
	if err != nil {
		return h.send(view.Error("Error deleting group."))
	}
	if !found {
		return h.send(fmt.Sprintf("Group *%s* not found.", name))
	}
	return h.send(view.Deleted("", name, "group"))
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
			if d, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && d > 0 && d <= models.MaxBudgetIntervalDays {
				return d, 0
			}
		}
		return 0, 0
	}
}
