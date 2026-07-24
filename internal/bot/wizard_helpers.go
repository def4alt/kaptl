package bot

import (
	"fmt"

	tele "gopkg.in/telebot.v4"
)

// ─── Wizard state helpers ──────────────────────────────────

// startWizard initializes a wizard state with template tracking.
func (b *Bot) startWizard(uid int64, c tele.Context, step WizardStep, txType, prevStep string) *userState {
	s := &userState{
		Step:          step,
		
		MsgID:  c.Message().ID,
		ChatID:        c.Chat().ID,
		Prev:   prevStep,
	}
	b.setState(uid, s)
	return s
}

// editStep edits the wizard's template message with new fields and keyboard.
func (b *Bot) editStep(state *userState, title string, fields map[string]string, markup *tele.ReplyMarkup) error {
	return b.editTemplate(state, progressTemplate(title, fields), markup)
}

// finishWizard clears state and edits the template to the final message.
func (b *Bot) finishWizard(c tele.Context, state *userState, title string, fields map[string]string) error {
	b.clearState(c.Sender().ID)
	text := progressTemplate(title, fields)
	return c.Edit("✅ Done!\n\n"+text, mainMenu())
}

// wizardFields is a map builder for the template display.
type wizardFields map[string]string

func (f wizardFields) with(key, value string) wizardFields {
	f[key] = value
	return f
}

func (f wizardFields) withDashed() wizardFields {
	dashed := map[string]string{
		"Category": "—", "Amount": "—", "Account": "—",
		"From": "—", "To": "—", "Emoji": "—", "Name": "—",
		"Currency": "—", "Group": "—", "Interval": "—",
	}
	for k, v := range dashed {
		if _, ok := f[k]; !ok {
			f[k] = v
		}
	}
	return f
}

// expenseFields builds fields for the expense/income template.
func expenseFields(cat, amount, acc string) wizardFields {
	return wizardFields{"Category": cat, "Amount": amount, "Account": acc}
}

func incomeFields(amount, acc string) wizardFields {
	return wizardFields{"Amount": amount, "Account": acc}
}

func transferFields(from, to, amount string) wizardFields {
	return wizardFields{"From": from, "To": to, "Amount": amount}
}

// ─── Convenience senders ───────────────────────────────────

func sendf(c tele.Context, format string, args ...interface{}) error {
	return c.Send(fmt.Sprintf(format, args...), mainMenu())
}

func editf(c tele.Context, format string, args ...interface{}) error {
	return c.Edit(fmt.Sprintf(format, args...), mainMenu())
}
