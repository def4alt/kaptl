package bot

import (
	"context"
	"log"
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Manage menu ──────────────────────────────────────────

func (b *Bot) handleManageMenu(c tele.Context) error {
	return c.Edit("⚙️ *Manage*", manageMenu())
}

func (b *Bot) handleManageCats(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, userID)
	groups, _ := b.Store.GetGroups(ctx, userID)
	groupMap := map[int64]models.CategoryGroup{}
	for _, g := range groups {
		groupMap[g.ID] = g
	}

	var lines []string
	for _, cat := range cats {
		gs := ""
		if cat.GroupID != nil {
			if g, ok := groupMap[*cat.GroupID]; ok {
				gs = fmt.Sprintf("  _%s %s_", g.Emoji, g.Name)
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s%s", cat.Emoji, cat.Name, gs))
	}
	if len(lines) == 0 {
		lines = append(lines, "_No categories_")
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Category", cbEmoji, "new_cat")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)

	return c.Edit("🏷️ *Categories*\n\n"+strings.Join(lines, "\n")+"\n\n`/cat add 🍞 Name`  `/cat rm Name`", menu)
}

func (b *Bot) handleManageAccs(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, userID)

	var lines []string
	for _, a := range accs {
		lines = append(lines, fmt.Sprintf("%s %s: %.2f %s", a.Emoji, a.Name, a.Balance, a.Currency))
	}
	if len(lines) == 0 {
		lines = append(lines, "_No accounts_")
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Account", cbEmoji, "new_acc")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)

	return c.Edit("💰 *Accounts*\n\n"+strings.Join(lines, "\n")+"\n\n`/acc add 💳 Name [currency]`", menu)
}

func (b *Bot) handleManageBuds(c tele.Context) error { return b.handleBudgetMenu(c) }

func (b *Bot) handleManageGrps(c tele.Context) error {
	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	groups, _ := b.Store.GetGroups(ctx, userID)

	var lines []string
	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("%s %s", g.Emoji, g.Name))
	}
	if len(lines) == 0 {
		lines = append(lines, "_No groups_")
	}

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Group", cbEmoji, "new_group")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)

	return c.Edit("📁 *Groups*\n\n"+strings.Join(lines, "\n")+"\n\n`/group add 📁 Name`  `/group rm Name`", menu)
}

// ─── Back button ──────────────────────────────────────────

func (b *Bot) handleBackBtn(c tele.Context) error {
	userID := c.Sender().ID
	defer c.Respond()

	state := b.stateFor(userID)
	if state == nil {
		return c.Edit("No active operation.", mainMenu())
	}

	switch state.PrevStep {
	case "pick_category":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		cats, _ := b.Store.GetCategories(ctx, userID)
		return c.Edit("*Pick a category:*", categoryKeyboard(cats))

	case "pick_move_source", "move_start":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		accs, _ := b.Store.GetAccounts(ctx, userID)
		state.Step = "awaiting_move_source"
		text := "🔀 *Transfer*\n\nFrom: —\nTo: —\nAmount: —"
		return c.Edit(text, accountKeyboard(accs))

	case "pick_move_target":
		state.Step = "awaiting_move_target"
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		accs, _ := b.Store.GetAccounts(ctx, userID)
		srcAcc, _ := b.Store.GetAccount(ctx, state.AccountID)
		src := "—"
		if srcAcc != nil {
			src = srcAcc.Emoji + " " + srcAcc.Name
		}
		text := fmt.Sprintf("🔀 *Transfer*\n\nFrom: %s\nTo: —\nAmount: —", src)
		return c.Edit(text, accountKeyboardExclude(accs, state.AccountID))
	}

	return c.Respond(&tele.CallbackResponse{Text: "Can't go back"})
}

// ─── Emoji picker ─────────────────────────────────────────

func (b *Bot) handleEmojiPick(c tele.Context) error {
	data := c.Callback().Data
	userID := c.Sender().ID
	defer c.Respond()

	switch data {
	case "new_cat":
		b.setState(userID, &userState{
			Step: "awaiting_cat_emoji", TxType: "new_cat",
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("🏷️ *New Category*\n\nEmoji: —\nName: —\nGroup: —\n\n_Pick an emoji:_", emojiKeyboard())

	case "new_acc":
		b.setState(userID, &userState{
			Step: "awaiting_acc_emoji", TxType: "new_acc",
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("💰 *New Account*\n\nEmoji: —\nName: —\nCurrency: —\n\n_Pick an emoji:_", emojiKeyboard())

	case "new_group":
		b.setState(userID, &userState{
			Step: "awaiting_group_emoji", TxType: "new_group",
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("📁 *New Group*\n\nEmoji: —\nName: —\n\n_Pick an emoji:_", emojiKeyboard())

	default:
		// Actual emoji picked
		state := b.stateFor(userID)
		if state == nil {
			return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
		}
		state.Description = data // store emoji

		switch state.Step {
		case "awaiting_cat_emoji":
			state.Step = "awaiting_cat_name"
			return c.Edit("🏷️ *New Category*\n\nEmoji: "+data+"\nName: —\nGroup: —\n\n_Type the category name:_", cancelBtn())
		case "awaiting_acc_emoji":
			state.Step = "awaiting_acc_name"
			return c.Edit("💰 *New Account*\n\nEmoji: "+data+"\nName: —\nCurrency: —\n\n_Type the account name:_", cancelBtn())
		case "awaiting_group_emoji":
			state.Step = "awaiting_group_name"
			return c.Edit("📁 *New Group*\n\nEmoji: "+data+"\nName: —\n\n_Type the group name:_", cancelBtn())
		}
	}
	return c.Respond(&tele.CallbackResponse{Text: "Done"})
}

// ─── Currency picker ──────────────────────────────────────

func (b *Bot) handleCurrencyPick(c tele.Context) error {
	userID := c.Sender().ID
	state := b.stateFor(userID)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	currency := c.Callback().Data
	parts := strings.SplitN(state.Description, "|", 2)
	emoji := parts[0]
	name := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	acc, err := b.Store.CreateAccount(ctx, userID, name, emoji, currency, 0)
	if err != nil {
		return c.Edit("❌ Error creating account.", manageMenu())
	}

	b.clearState(userID)
	return c.Edit(fmt.Sprintf("✅ Created: %s *%s* (%s)", acc.Emoji, acc.Name, acc.Currency), manageMenu())
}

// ─── Interval picker ──────────────────────────────────────

func (b *Bot) handleIntervalPick(c tele.Context) error {
	userID := c.Sender().ID
	state := b.stateFor(userID)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	interval := c.Callback().Data
	intervalDays, intervalMonths := parseInterval(interval)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	bd, err := b.Store.SetBudget(ctx, userID, state.EditingBudget, intervalDays, intervalMonths, state.Amount)
	if err != nil {
		log.Printf("set budget: %v", err)
		return c.Edit("❌ Error saving budget.", manageMenu())
	}

	b.clearState(userID)

	cats, _ := b.Store.GetCategories(ctx, userID)
	catName := "category"
	for _, cat := range cats {
		if cat.ID == state.EditingBudget {
			catName = cat.Emoji + " " + cat.Name
			break
		}
	}

	return c.Edit(fmt.Sprintf("✅ Budget: %s *%.0f* (%s)", catName, state.Amount, bd.Description()), manageMenu())
}

// ─── Group picker ─────────────────────────────────────────

func (b *Bot) handleGroupPick(c tele.Context) error {
	userID := c.Sender().ID
	state := b.stateFor(userID)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	groupIDStr := c.Callback().Data
	var groupID *int64
	if groupIDStr != "0" {
		id, _ := strconv.ParseInt(groupIDStr, 10, 64)
		groupID = &id
	}

	parts := strings.SplitN(state.Description, "|", 2)
	emoji := parts[0]
	name := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	cat, err := b.Store.CreateCategory(ctx, userID, name, emoji, groupID)
	if err != nil {
		return c.Edit("❌ Category already exists.", manageMenu())
	}

	b.clearState(userID)
	return c.Edit(fmt.Sprintf("✅ Created: %s *%s*", cat.Emoji, cat.Name), manageMenu())
}

// ─── Name receivers ───────────────────────────────────────

func (b *Bot) receiveCatName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}

	state.Description = state.Description + "|" + name // emoji|name
	state.Step = "awaiting_cat_group"

	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	groups, _ := b.Store.GetGroups(ctx, userID)

	return c.Send("🏷️ *New Category*\n\nEmoji: "+state.Description[:len(state.Description)-len(name)-1]+"\nName: "+name+"\nGroup: —\n\n_Pick a group:_", groupPickerKeyboard(groups))
}

func (b *Bot) receiveAccName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}

	state.Description = state.Description + "|" + name // emoji|name
	state.Step = "awaiting_acc_currency"

	return c.Send("💰 *New Account*\n\nEmoji: "+state.Description[:len(state.Description)-len(name)-1]+"\nName: "+name+"\nCurrency: —\n\n_Pick currency:_", currencyKeyboard())
}

func (b *Bot) receiveGroupName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}

	userID := c.Sender().ID
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	g, err := b.Store.CreateGroup(ctx, userID, name, state.Description)
	if err != nil {
		return c.Send("❌ Group already exists.", manageMenu())
	}

	b.clearState(userID)
	return c.Send(fmt.Sprintf("✅ Created group: %s *%s*", g.Emoji, g.Name), manageMenu())
}
