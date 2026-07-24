package bot

import (
	tele "gopkg.in/telebot.v4"
)

func (b *Bot) startWizard(uid int64, c tele.Context, step WizardStep, w Wizard, prev string) *userState {
	s := &userState{
		Wizard: w,
		Step:   step,
		MsgID:  c.Message().ID,
		ChatID: c.Chat().ID,
		Prev:   prev,
	}
	b.setState(uid, s)
	return s
}
