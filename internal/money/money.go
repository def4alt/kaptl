package money

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

var exponents = map[string]int32{
	"EUR": 2,
	"GBP": 2,
	"PLN": 2,
	"UAH": 2,
	"USD": 2,
}

type Money struct {
	Amount   decimal.Decimal
	Currency string
}

func NormalizeCurrency(value string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(value))
	if _, ok := exponents[code]; !ok {
		return "", fmt.Errorf("unsupported currency %q", value)
	}
	return code, nil
}

func Exponent(currency string) (int32, error) {
	code, err := NormalizeCurrency(currency)
	if err != nil {
		return 0, err
	}
	return exponents[code], nil
}

func Parse(value, currency string) (Money, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return Money{}, fmt.Errorf("invalid amount: %w", err)
	}
	code, err := NormalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	exponent := exponents[code]
	if !amount.Equal(amount.Truncate(exponent)) {
		return Money{}, fmt.Errorf("%s supports at most %d decimal places", code, exponent)
	}
	return Money{Amount: amount, Currency: code}, nil
}

func New(amount decimal.Decimal, currency string) (Money, error) {
	code, err := NormalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}
	return Money{Amount: amount, Currency: code}, nil
}

func (m Money) Round() Money {
	exponent, ok := exponents[m.Currency]
	if !ok {
		return m
	}
	m.Amount = m.Amount.RoundBank(exponent)
	return m
}
