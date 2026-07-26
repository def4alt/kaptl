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

func TestValuationMigrationKeepsNativeLedgerAndQueuesReportableTransactions(t *testing.T) {
	sql := migrationSQL(t, "003_reporting_valuations.sql")
	for _, invariant := range []string{
		"reporting_currency",
		"CREATE TABLE IF NOT EXISTS fx_quotes",
		"CREATE TABLE IF NOT EXISTS transaction_valuations",
		"CREATE TABLE IF NOT EXISTS valuation_jobs",
		"REFERENCES transactions(id) ON DELETE CASCADE",
		"CREATE TRIGGER transactions_enqueue_valuation",
		"INSERT INTO valuation_jobs",
		"CHECK (reporting_currency = 'EUR')",
		"status TEXT NOT NULL DEFAULT 'pending'",
		"locked_token TEXT",
		"round_half_even_2",
		"valuation amount does not match native amount and quote rate",
		"transaction_valuations_policy_shape_check",
		"valuation_jobs_policy_shape_check",
		"valuation_jobs_terminal_state_check",
		"rate NUMERIC(38,20)",
		"NEW.type IN ('expense', 'income')",
		"quote currencies do not match transaction valuation",
		"FX quotes are immutable",
		"budget currency must equal user reporting currency",
		"transaction valuation inputs are immutable",
		"NEW.category_id IS DISTINCT FROM OLD.category_id",
		"reporting currency is immutable",
	} {
		if !strings.Contains(sql, invariant) {
			t.Errorf("003_reporting_valuations.sql lacks %q", invariant)
		}
	}
	if strings.Contains(sql, "ALTER TABLE transactions DROP COLUMN amount") || strings.Contains(sql, "UPDATE transactions SET amount") {
		t.Error("valuation migration mutates native transaction amounts")
	}
}
