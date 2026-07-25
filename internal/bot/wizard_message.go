package bot

import (
	"fmt"

	tele "gopkg.in/telebot.v4"
)

// editWizardMessage updates the original message tracked by a wizard after a
// free-text reply. Callback handlers should edit their current context directly.
func (b *Bot) editWizardMessage(state *userState, text string, markup *tele.ReplyMarkup) error {
	if state.MsgID == 0 || state.ChatID == 0 {
		return fmt.Errorf("wizard has no message to edit")
	}

	message := &tele.Message{
		ID:   state.MsgID,
		Chat: &tele.Chat{ID: state.ChatID},
	}
	_, err := b.Tele.Edit(message, text, markup)
	return err
}
