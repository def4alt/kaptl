package bot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModernArchitectureHasNoLegacyAdapters(t *testing.T) {
	legacyFiles := []string{
		"views.go",
		"wizard_helpers.go",
		filepath.Join("view", "keyboards.go"),
		filepath.Join("..", "models", "store.go"),
	}
	for _, path := range legacyFiles {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy path %q must not exist", path)
		}
	}
}

func TestViewPackageStaysTelegramIndependent(t *testing.T) {
	entries, err := os.ReadDir("view")
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		content, err := os.ReadFile(filepath.Join("view", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "gopkg.in/telebot") {
			t.Errorf("view/%s imports Telegram runtime code", entry.Name())
		}
	}
}

func TestInitialMigrationHasNoRemovedBudgetMonthColumn(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "budgets(user_id, month)") {
		t.Fatal("initial migration indexes removed budgets.month column")
	}
}
