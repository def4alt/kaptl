package view

import (
	"fmt"
)

// ─── Progress template (wizard in-place editing) ───────────

func ProgressTemplate(title string, fields map[string]string) string {
	order := []string{"Category", "Amount", "Account", "From", "To", "Emoji", "Name", "Currency", "Group", "Interval"}
	msg := title + "\n\n"
	for _, key := range order {
		if val, ok := fields[key]; ok {
			msg += fmt.Sprintf("%s: %s\n", key, val)
		}
	}
	return msg
}

// ─── Wizard field builders ─────────────────────────────────

type Fields map[string]string

func (f Fields) With(key, value string) Fields {
	f[key] = value
	return f
}

func ExpenseFields(cat, amount, acc string) Fields {
	return Fields{"Category": cat, "Amount": amount, "Account": acc}
}

func IncomeFields(amount, acc string) Fields {
	return Fields{"Amount": amount, "Account": acc}
}

func TransferFields(from, to, amount string) Fields {
	return Fields{"From": from, "To": to, "Amount": amount}
}
