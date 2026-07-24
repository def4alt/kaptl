package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v4"
)

// ─── Menus ─────────────────────────────────────────────────

func (b *Bot) handleManageMenu(c tele.Context) error {
	return c.Edit("⚙️ *Manage*", manageMenu())
}

func (b *Bot) handleManageCats(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cats, _ := b.Store.GetCategories(ctx, c.Sender().ID)
	groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Category", cbEmoji, "new_cat")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgCategories(cats, groups), menu)
}

func (b *Bot) handleManageAccs(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	accs, _ := b.Store.GetAccounts(ctx, c.Sender().ID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Account", cbEmoji, "new_acc")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgAccounts(accs), menu)
}

func (b *Bot) handleManageBuds(c tele.Context) error { return b.handleBudgetMenu(c) }

func (b *Bot) handleManageGrps(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)

	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Group", cbEmoji, "new_group")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgGroups(groups), menu)
}

// ─── Summary / Recent ─────────────────────────────────────

func (b *Bot) handleSummary(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	rows, _ := b.Store.GetBudgetSummary(ctx, c.Sender().ID, 0)
	if len(rows) == 0 {
		return c.Send("No categories yet. Use `/cat add 🍞 Name`.", mainMenu())
	}
	rta, _ := b.Store.GetReadyToAssign(ctx, c.Sender().ID)
	return c.Send(msgSummary(rows, rta), mainMenu())
}

func (b *Bot) handleRecent(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	txs, _ := b.Store.GetRecentTransactions(ctx, c.Sender().ID, 10)
	return c.Send(msgRecent(txs), mainMenu())
}

// ─── Cancel ───────────────────────────────────────────────

func (b *Bot) handleCancel(c tele.Context) error {
	b.clearState(c.Sender().ID)
	return c.Send("❌ Cancelled.", mainMenu())
}

func (b *Bot) handleDynamicCancel(c tele.Context) error {
	defer c.Respond()
	b.clearState(c.Sender().ID)
	return c.Edit("❌ Cancelled.", mainMenu())
}

// ─── Back button ──────────────────────────────────────────

func (b *Bot) handleBackBtn(c tele.Context) error {
	uid := c.Sender().ID
	defer c.Respond()
	state := b.stateFor(uid)
	if state == nil {
		return c.Edit("No active operation.", mainMenu())
	}

	switch state.PrevStep {
	case "pick_category":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		cats, _ := b.Store.GetCategories(ctx, uid)
		return c.Edit("*Pick a category:*", categoryKeyboard(cats))

	case "move_start":
		ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
		defer cancel()
		accs, _ := b.Store.GetAccounts(ctx, uid)
		state.Step = StepAwaitMoveSource
		return c.Edit("🔀 *Transfer*\n\nFrom: —\nTo: —\nAmount: —", accountKeyboard(accs))
	}
	return c.Respond(&tele.CallbackResponse{Text: "Can't go back"})
}

// ─── Interactive creation ─────────────────────────────────

func (b *Bot) handleEmojiPick(c tele.Context) error {
	data := c.Callback().Data
	uid := c.Sender().ID
	defer c.Respond()

	switch data {
	case "new_cat":
		b.setState(uid, &userState{
			Step:          StepAwaitCatEmoji,
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("🏷️ *New Category*\n\nEmoji: —\nName: —\nGroup: —\n\n_Pick an emoji:_", emojiKeyboard())

	case "new_acc":
		b.setState(uid, &userState{
			Step:          StepAwaitAccEmoji,
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("💰 *New Account*\n\nEmoji: —\nName: —\nCurrency: —\n\n_Pick an emoji:_", emojiKeyboard())

	case "new_group":
		b.setState(uid, &userState{
			Step:          StepAwaitGroupEmoji,
			TemplateMsgID: c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("📁 *New Group*\n\nEmoji: —\nName: —\n\n_Pick an emoji:_", emojiKeyboard())

	default:
		state := b.stateFor(uid)
		if state == nil {
			return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
		}
		state.Emoji = data

		switch state.Step {
		case StepAwaitCatEmoji:
			state.Step = StepAwaitCatName
			return c.Edit(fmt.Sprintf("🏷️ *New Category*\n\nEmoji: %s\nName: —\nGroup: —\n\n_Type the name:_", data), cancelBtn())
		case StepAwaitAccEmoji:
			state.Step = StepAwaitAccName
			return c.Edit(fmt.Sprintf("💰 *New Account*\n\nEmoji: %s\nName: —\nCurrency: —\n\n_Type the name:_", data), cancelBtn())
		case StepAwaitGroupEmoji:
			state.Step = StepAwaitGroupName
			return c.Edit(fmt.Sprintf("📁 *New Group*\n\nEmoji: %s\nName: —\n\n_Type the name:_", data), cancelBtn())
		}
	}
	return c.Respond(&tele.CallbackResponse{Text: "Done"})
}

func (b *Bot) receiveCatName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}
	state.Name = name
	state.Step = StepAwaitCatGroup

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	groups, _ := b.Store.GetGroups(ctx, c.Sender().ID)
	return c.Send(fmt.Sprintf("🏷️ *New Category*\n\nEmoji: %s\nName: %s\nGroup: —\n\n_Pick a group:_", state.Emoji, name), groupPickerKeyboard(groups))
}

func (b *Bot) receiveAccName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}
	state.Name = name
	state.Step = StepAwaitAccCurrency
	return c.Send(fmt.Sprintf("💰 *New Account*\n\nEmoji: %s\nName: %s\nCurrency: —\n\n_Pick currency:_", state.Emoji, name), currencyKeyboard())
}

func (b *Bot) receiveGroupName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if name == "" || len(name) > 100 {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	g, err := b.Store.CreateGroup(ctx, c.Sender().ID, name, state.Emoji)
	if err != nil {
		return c.Send("❌ Group already exists.", manageMenu())
	}
	b.clearState(c.Sender().ID)
	return c.Send(fmt.Sprintf("✅ Created group: %s *%s*", g.Emoji, g.Name), manageMenu())
}

// ─── Currency / Interval / Group pickers ──────────────────

func (b *Bot) handleCurrencyPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}
	currency := c.Callback().Data

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	acc, err := b.Store.CreateAccount(ctx, uid, state.Name, state.Emoji, currency, 0)
	if err != nil {
		return c.Edit("❌ Error creating account.", manageMenu())
	}
	b.clearState(uid)
	return c.Edit(fmt.Sprintf("✅ Created: %s *%s* (%s)", acc.Emoji, acc.Name, acc.Currency), manageMenu())
}

func (b *Bot) handleIntervalPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}
	interval := c.Callback().Data
	d, m := parseInterval(interval)

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	bd, err := b.Store.SetBudget(ctx, uid, state.EditingBudget, d, m, state.Amount)
	if err != nil {
		return c.Edit("❌ Error saving budget.", manageMenu())
	}
	b.clearState(uid)
	cats, _ := b.Store.GetCategories(ctx, uid)
	return c.Edit(fmt.Sprintf("✅ Budget: %s *%.0f* (%s)", findCatName(cats, state.EditingBudget), state.Amount, bd.Description()), manageMenu())
}

func (b *Bot) handleGroupPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	groupIDStr := c.Callback().Data
	var groupID *int64
	if groupIDStr != "0" {
		id, _ := strconv.ParseInt(groupIDStr, 10, 64)
		groupID = &id
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	cat, err := b.Store.CreateCategory(ctx, uid, state.Name, state.Emoji, groupID)
	if err != nil {
		return c.Edit("❌ Category already exists.", manageMenu())
	}
	b.clearState(uid)
	return c.Edit(fmt.Sprintf("✅ Created: %s *%s*", cat.Emoji, cat.Name), manageMenu())
}
