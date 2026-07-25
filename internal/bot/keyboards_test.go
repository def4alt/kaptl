package bot

import (
	"testing"

	"github.com/def4alt/kaptl/internal/models"
)

func TestAccountKeyboardShowsBackOnlyForSupportedSteps(t *testing.T) {
	accounts := []models.Account{{ID: 1, Emoji: "💳", Name: "Checking"}}

	withoutBack := accountKeyboard(accounts, false)
	footer := withoutBack.InlineKeyboard[len(withoutBack.InlineKeyboard)-1]
	if len(footer) != 1 || footer[0].Text != "❌ Cancel" {
		t.Fatalf("unsupported back path rendered: %#v", footer)
	}

	withBack := accountKeyboard(accounts, true)
	footer = withBack.InlineKeyboard[len(withBack.InlineKeyboard)-1]
	if len(footer) != 2 || footer[0].Text != "◀ Back" || footer[1].Text != "❌ Cancel" {
		t.Fatalf("supported back path missing: %#v", footer)
	}
}

func TestManageBackButtonsHaveDistinctRoutes(t *testing.T) {
	mainBack := manageMenu().InlineKeyboard[2][0]
	if mainBack.CallbackUnique() != "\f"+btnBackMain.Unique {
		t.Fatalf("manage menu back route = %q, want %q", mainBack.CallbackUnique(), "\f"+btnBackMain.Unique)
	}

	manageBack := manageCategoryMenu().InlineKeyboard[1][0]
	if manageBack.CallbackUnique() != "\f"+btnBackManage.Unique {
		t.Fatalf("resource menu back route = %q, want %q", manageBack.CallbackUnique(), "\f"+btnBackManage.Unique)
	}
}
