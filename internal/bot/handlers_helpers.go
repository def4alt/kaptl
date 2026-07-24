package bot

import (
	"fmt"

	tele "gopkg.in/telebot.v4"
)

// ─── Progress template helpers ────────────────────────────

// progressTemplate builds a multi-line template showing wizard progress.
func progressTemplate(title string, fields map[string]string) string {
	order := []string{"Category", "Amount", "Account", "From", "To"}
	msg := title + "\n\n"
	for _, key := range order {
		if val, ok := fields[key]; ok {
			msg += fmt.Sprintf("%s: %s\n", key, val)
		}
	}
	return msg
}

// editTemplate edits the stored template message in-place.
func (b *Bot) editTemplate(state *userState, text string, markup *tele.ReplyMarkup) error {
	if state.TemplateMsgID == 0 || state.ChatID == 0 {
		return fmt.Errorf("no template message")
	}
	msg := &tele.Message{ID: state.TemplateMsgID, Chat: &tele.Chat{ID: state.ChatID}}
	_, err := b.Tele.Edit(msg, text, markup)
	return err
}
