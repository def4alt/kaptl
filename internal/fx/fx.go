package fx

import (
	"fmt"
	"time"

	"github.com/def4alt/kaptl/internal/money"
	"github.com/shopspring/decimal"
)

const PurposeBudget = "budget"

type Quote struct {
	ID          int64
	Source      string
	Target      string
	Rate        decimal.Decimal
	EffectiveAt time.Time
	ObservedAt  time.Time
	Provider    string
}

func Convert(source money.Money, quote Quote) (money.Money, error) {
	sourceCode, err := money.NormalizeCurrency(quote.Source)
	if err != nil {
		return money.Money{}, err
	}
	if source.Currency != sourceCode {
		return money.Money{}, fmt.Errorf("quote source %s does not match money currency %s", sourceCode, source.Currency)
	}
	if !quote.Rate.IsPositive() {
		return money.Money{}, fmt.Errorf("exchange rate must be positive")
	}
	converted, err := money.New(source.Amount.Mul(quote.Rate), quote.Target)
	if err != nil {
		return money.Money{}, err
	}
	return converted.Round(), nil
}
