package models

import (
	"fmt"
	"strings"
)

var supportedCurrencies = map[string]struct{}{
	"EUR": {},
	"GBP": {},
	"PLN": {},
	"UAH": {},
	"USD": {},
}

// NormalizeCurrency returns a supported canonical ISO 4217 code.
// Kaptl currently supports currencies with two decimal places only.
func NormalizeCurrency(value string) (string, error) {
	currency := strings.ToUpper(strings.TrimSpace(value))
	if _, ok := supportedCurrencies[currency]; !ok {
		return "", fmt.Errorf("unsupported currency %q", value)
	}
	return currency, nil
}
