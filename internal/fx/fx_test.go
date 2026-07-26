package fx_test

import (
	"testing"
	"time"

	"github.com/def4alt/kaptl/internal/fx"
	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

func TestConvertAppliesExplicitSourceToTargetDirection(t *testing.T) {
	source, err := money.Parse("721.00", "UAH")
	if err != nil {
		t.Fatal(err)
	}
	quote := fx.Quote{
		Source:      "UAH",
		Target:      "EUR",
		Rate:        decimal.RequireFromString("0.02085992"),
		EffectiveAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC),
		Provider:    "ECB",
	}

	got, err := fx.Convert(source, quote)
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != "EUR" || got.Amount.StringFixed(2) != "15.04" {
		t.Fatalf("got %s %s", got.Currency, got.Amount)
	}
}

func TestConvertRejectsQuoteForDifferentSourceCurrency(t *testing.T) {
	source, _ := money.Parse("10", "USD")
	_, err := fx.Convert(source, fx.Quote{Source: "UAH", Target: "EUR", Rate: decimal.NewFromInt(1)})
	if err == nil {
		t.Fatal("expected currency mismatch")
	}
}
