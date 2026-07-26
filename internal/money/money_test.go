package money_test

import (
	"testing"

	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

func TestParseRejectsFractionsSmallerThanCurrencyMinorUnit(t *testing.T) {
	if _, err := money.Parse("721.005", "UAH"); err == nil {
		t.Fatal("expected excess precision to be rejected")
	}
}

func TestParseNormalizesCurrencyAndPreservesDecimalValue(t *testing.T) {
	got, err := money.Parse(" 1071.20 ", " uah ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != "UAH" || !got.Amount.Equal(decimal.RequireFromString("1071.20")) {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizeCurrencyRejectsUnsupportedCodes(t *testing.T) {
	for _, code := range []string{"JPY", "", "EURO"} {
		if _, err := money.NormalizeCurrency(code); err == nil {
			t.Fatalf("NormalizeCurrency(%q) unexpectedly succeeded", code)
		}
	}
}

func TestRoundUsesTargetCurrencyExponentAndHalfEven(t *testing.T) {
	got, err := money.New(decimal.RequireFromString("15.045"), "EUR")
	if err != nil {
		t.Fatal(err)
	}
	got = got.Round()
	if got.Amount.StringFixed(2) != "15.04" {
		t.Fatalf("got %s, want 15.04", got.Amount)
	}
}
