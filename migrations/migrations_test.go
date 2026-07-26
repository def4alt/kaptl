package migrations

import (
	"os"
	"strings"
	"testing"
)

func migrationSQL(t *testing.T, name string) string {
	t.Helper()
	contents, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestInitialMigrationTriggersAreIdempotent(t *testing.T) {
	sql := migrationSQL(t, "001_init.sql")
	for _, trigger := range []string{
		"transactions_currency_guard ON transactions",
		"accounts_currency_guard ON accounts",
		"budgets_ownership_guard ON budgets",
		"categories_group_ownership_guard ON categories",
	} {
		if !strings.Contains(sql, "DROP TRIGGER IF EXISTS "+trigger) {
			t.Errorf("001_init.sql does not idempotently replace %s", trigger)
		}
	}
}

func TestMultiCurrencyMigrationScopesConstraintChecks(t *testing.T) {
	sql := migrationSQL(t, "002_multi_currency.sql")
	for _, relation := range []string{"accounts", "transactions", "budgets"} {
		if !strings.Contains(sql, "conrelid = '"+relation+"'::regclass") {
			t.Errorf("constraint checks are not scoped to %s", relation)
		}
	}
}

func TestMigrationsEnforceCategoryGroupOwnership(t *testing.T) {
	for _, name := range []string{"001_init.sql", "002_multi_currency.sql"} {
		sql := migrationSQL(t, name)
		if !strings.Contains(sql, "category group does not belong to category user") {
			t.Errorf("%s lacks category group ownership trigger", name)
		}
	}
	if !strings.Contains(migrationSQL(t, "002_multi_currency.sql"), "category group ownership mismatch found") {
		t.Error("002_multi_currency.sql lacks category group ownership audit")
	}
}

func TestMigrationsMakeAccountCurrencyUnconditionallyImmutable(t *testing.T) {
	for _, name := range []string{"001_init.sql", "002_multi_currency.sql"} {
		sql := migrationSQL(t, name)
		if !strings.Contains(sql, "account currency is immutable") {
			t.Errorf("%s lacks immutable account currency guard", name)
		}
		if strings.Contains(sql, "OLD.initial_balance <> 0 OR EXISTS") {
			t.Errorf("%s still permits racing account currency updates", name)
		}
	}
}
