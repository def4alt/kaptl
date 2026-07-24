package bot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/def4alt/kaptl/internal/bot/view"
	tele "gopkg.in/telebot.v4"
)

// ─── Menus ─────────────────────────────────────────────────

func (b *Bot) handleManageMenu(c tele.Context) error {
	return c.Edit("⚙️ *Manage*", manageMenu())
}

func (b *Bot) handleManageCats(c tele.Context) error {
	h := b.withCtx(c); defer h.done()
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Category", cbEmoji, "new_cat")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgCategories(h.cats(), h.groups()), menu)
}

func (b *Bot) handleManageAccs(c tele.Context) error {
	h := b.withCtx(c); defer h.done()
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Account", cbEmoji, "new_acc")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgAccounts(h.accs()), menu)
}

func (b *Bot) handleManageBuds(c tele.Context) error { return b.handleBudgetMenu(c) }

func (b *Bot) handleManageGrps(c tele.Context) error {
	h := b.withCtx(c); defer h.done()
	menu := &tele.ReplyMarkup{}
	menu.Inline(
		menu.Row(menu.Data("➕ Add Group", cbEmoji, "new_group")),
		menu.Row(menu.Data("◀ Back", "mg_back")),
	)
	return c.Edit(msgGroups(h.groups()), menu)
}

// ─── Summary / Recent ─────────────────────────────────────

func (b *Bot) handleSummary(c tele.Context) error {
	h := b.withCtx(c); defer h.done()
	rows, _ := h.Bot.Store.GetBudgetSummary(h.DB, h.UID, 0)
	if len(rows) == 0 {
		return h.send("No categories yet. Use `/cat add 🍞 Name`.")
	}
	rta, _ := h.Bot.Store.GetReadyToAssign(h.DB, h.UID)
	return h.send(view.Summary(rows, rta))
}

func (b *Bot) handleRecent(c tele.Context) error {
	h := b.withCtx(c); defer h.done()
	txs, _ := h.Bot.Store.GetRecentTransactions(h.DB, h.UID, 10)
	return h.send(msgRecent(txs))
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

	switch state.Prev {
	case "pick_category":
		h := b.withCtx(c); defer h.done()
		return c.Edit("*Pick a category:*", categoryKeyboard(h.cats()))
	case "move_start":
		h := b.withCtx(c); defer h.done()
		state.Step = StepMoveSource
		return c.Edit("🔀 *Transfer*\n\nFrom: —\nTo: —\nAmount: —", accountKeyboard(h.accs()))
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
			Wizard: CreationWizard{Kind: "cat"},
			Step:   StepCreateEmoji,
			MsgID:  c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("🏷️ *New Category*\n\nEmoji: —\nName: —\nGroup: —\n\n_Pick an emoji:_", emojiKeyboard())
	case "new_acc":
		b.setState(uid, &userState{
			Wizard: CreationWizard{Kind: "acc"},
			Step:   StepCreateEmoji,
			MsgID:  c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("💰 *New Account*\n\nEmoji: —\nName: —\nCurrency: —\n\n_Pick an emoji:_", emojiKeyboard())
	case "new_group":
		b.setState(uid, &userState{
			Wizard: CreationWizard{Kind: "group"},
			Step:   StepCreateEmoji,
			MsgID:  c.Message().ID, ChatID: c.Chat().ID,
		})
		return c.Edit("📁 *New Group*\n\nEmoji: —\nName: —\n\n_Pick an emoji:_", emojiKeyboard())
	default:
		state := b.stateFor(uid)
		if state == nil {
			return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
		}
		w := state.creationW()
		w.Emoji = data
		state.Wizard = w
		state.Step = StepCreateName

		switch {
		case state.Prev == "" && state.Step == StepCreateName:
			// Determine context from data (new_cat/new_acc/new_group)
			return c.Edit(fmt.Sprintf("Emoji: %s\n\n_Type the name:_", data), cancelBtn())
		default:
			return c.Edit(fmt.Sprintf("Emoji: %s\nName: —\n\n_Type the name:_", data), cancelBtn())
		}
	}
}

// receiveCreateName is the unified name receiver for all creation wizards.
// It dispatches to the next step based on the creation context.
func (b *Bot) receiveCreateName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if !isValidName(name) {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}

	w := state.creationW()
	w.Name = name
	state.Wizard = w

	h := b.withCtx(c); defer h.done()

	switch w.Kind {
	case "cat":
		state.Step = StepCreateGroup
		return c.Send(fmt.Sprintf("🏷️ *New Category*\n\nEmoji: %s\nName: %s\nGroup: —\n\n_Pick a group:_", w.Emoji, name), groupPickerKeyboard(h.groups()))
	case "acc":
		state.Step = StepCreateCurrency
		return c.Send(fmt.Sprintf("💰 *New Account*\n\nEmoji: %s\nName: %s\nCurrency: —\n\n_Pick currency:_", w.Emoji, name), currencyKeyboard())
	case "group":
		g, err := h.Bot.Store.CreateGroup(h.DB, h.UID, name, w.Emoji)
		if err != nil {
			return h.send(respondError("Group already exists."))
		}
		h.Bot.clearState(h.UID)
		return h.send(respondCreated(g.Emoji, g.Name, "group"))
	}
	return nil
}


func (b *Bot) receiveCatName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if !isValidName(name) {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}
	w := state.creationW()
	w.Name = name
	state.Wizard = w
	state.Step = StepCreateGroup

	h := b.withCtx(c); defer h.done()
	return c.Send(fmt.Sprintf("🏷️ *New Category*\n\nEmoji: %s\nName: %s\nGroup: —\n\n_Pick a group:_", w.Emoji, name), groupPickerKeyboard(h.groups()))
}

func (b *Bot) receiveGroupName(c tele.Context, state *userState) error {
	name := strings.TrimSpace(c.Text())
	if !isValidName(name) {
		return c.Send("Name must be 1-100 characters.", cancelBtn())
	}
	w := state.creationW()
	w.Name = name
	state.Wizard = w

	h := b.withCtx(c); defer h.done()
	g, err := h.Bot.Store.CreateGroup(h.DB, h.UID, name, w.Emoji)
	if err != nil {
		return h.send(respondError("Group already exists."))
	}
	h.Bot.clearState(h.UID)
	return h.send(respondCreated(g.Emoji, g.Name, "group"))
}

// ─── Picker callbacks (use variant access) ────────────────

func (b *Bot) handleCurrencyPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	w := state.creationW()
	currency := c.Callback().Data

	h := b.withCtx(c); defer h.done()
	acc, err := h.Bot.Store.CreateAccount(h.DB, uid, w.Name, w.Emoji, currency, 0)
	if err != nil {
		return h.edit(respondError("Error creating account."), manageMenu())
	}
	b.clearState(uid)
	return h.edit(fmt.Sprintf("✅ Created: %s *%s* (%s)", acc.Emoji, acc.Name, acc.Currency), manageMenu())
}

func (b *Bot) handleIntervalPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	w := state.budgetW()
	d, m := parseInterval(c.Callback().Data)

	h := b.withCtx(c); defer h.done()
	bd, err := h.Bot.Store.SetBudget(h.DB, uid, w.CategoryID, d, m, w.Amount)
	if err != nil {
		return h.edit(respondError("Error saving budget."), manageMenu())
	}
	b.clearState(uid)
	return h.edit(fmt.Sprintf("✅ Budget: %s *%.0f* (%s)", h.catName(w.CategoryID), w.Amount, bd.Description()), manageMenu())
}

func (b *Bot) handleGroupPick(c tele.Context) error {
	uid := c.Sender().ID
	state := b.stateFor(uid)
	if state == nil {
		return c.Respond(&tele.CallbackResponse{Text: "No active operation"})
	}

	w := state.creationW()
	groupIDStr := c.Callback().Data
	if groupIDStr != "0" {
		id, _ := strconv.ParseInt(groupIDStr, 10, 64)
		w.CatGroup = &id
		state.Wizard = w
	}

	h := b.withCtx(c); defer h.done()
	cat, err := h.Bot.Store.CreateCategory(h.DB, uid, w.Name, w.Emoji, w.CatGroup)
	if err != nil {
		return h.edit(respondError("Category already exists."), manageMenu())
	}
	b.clearState(uid)
	return h.edit(respondCreated(cat.Emoji, cat.Name, ""), manageMenu())
}
