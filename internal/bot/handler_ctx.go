package bot

import (
	"context"
	"strings"

	"github.com/def4alt/kaptl/internal/models"
	tele "gopkg.in/telebot.v4"
)

// ─── Handler context ──────────────────────────────────────

// hctx bundles the repeated boilerplate every handler needs.
type hctx struct {
	Bot    *Bot
	C      tele.Context
	UID    int64
	DB     context.Context
	cancel context.CancelFunc
}

// withCtx creates a handler context from a telebot Context.
func (b *Bot) withCtx(c tele.Context) *hctx {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	return &hctx{
		Bot:    b,
		C:      c,
		UID:    c.Sender().ID,
		DB:     ctx,
		cancel: cancel,
	}
}

// done cleans up the DB context.
func (h *hctx) done() { h.cancel() }

// send is a shorthand that always appends mainMenu.
func (h *hctx) send(text string) error {
	return h.C.Send(text, mainMenu())
}

// ─── Entity accessors ─────────────────────────────────────

// cats returns all categories for the current user.
func (h *hctx) cats() []models.Category {
	cats, _ := h.Bot.Store.GetCategories(h.DB, h.UID)
	return cats
}

// accs returns all accounts for the current user.
func (h *hctx) accs() []models.Account {
	accs, _ := h.Bot.Store.GetAccounts(h.DB, h.UID)
	return accs
}

// groups returns all category groups for the current user.
func (h *hctx) groups() []models.CategoryGroup {
	groups, _ := h.Bot.Store.GetGroups(h.DB, h.UID)
	return groups
}

// ─── Emoji parsing ────────────────────────────────────────

// parseEmojiArgs extracts an emoji from the first argument.
// Returns (emoji, remainingArgs). If no emoji found, returns (default, args).
func parseEmojiArgs(args []string, defaultEmoji string) (emoji string, rest []string) {
	if len(args) < 2 {
		// Not enough args for emoji+name; treat all as name
		return defaultEmoji, args
	}
	first := []rune(args[0])
	if len(first) <= 4 && first[0] > 127 {
		return args[0], args[1:]
	}
	return defaultEmoji, args
}

// ─── Delete-by-name helper ─────────────────────────────────

// deleteByName finds an entity by name (case-insensitive) and calls deleteFn.
// Returns true if found and deleted, false if not found.
func deleteByName[T any](items []T, name string, getName func(T) string, deleteFn func(T) error) (bool, error) {
	for _, item := range items {
		if strings.EqualFold(getName(item), name) {
			return true, deleteFn(item)
		}
	}
	return false, nil
}

// ─── Name validation ──────────────────────────────────────

func isValidName(name string) bool {
	n := strings.TrimSpace(name)
	return len(n) > 0 && len(n) <= 100
}
