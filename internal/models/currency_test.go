package models

import "testing"

func TestNormalizeCurrency(t *testing.T) {
	for input, want := range map[string]string{
		"eur":   "EUR",
		" UAH ": "UAH",
		"usd":   "USD",
		"pln":   "PLN",
		"gbp":   "GBP",
	} {
		got, err := NormalizeCurrency(input)
		if err != nil || got != want {
			t.Errorf("NormalizeCurrency(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	for _, input := range []string{"", "EU", "ABC", "EURO", "12A"} {
		if _, err := NormalizeCurrency(input); err == nil {
			t.Errorf("NormalizeCurrency(%q) unexpectedly succeeded", input)
		}
	}
}
