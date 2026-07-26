package models

import "fmt"

const (
	MaxBudgetIntervalDays   = 3650
	MaxBudgetIntervalMonths = 120
)

// ValidateBudgetInterval ensures rollover loops have a positive, finite period.
func ValidateBudgetInterval(days, months int) error {
	if days < 0 || months < 0 {
		return fmt.Errorf("budget interval components must be nonnegative")
	}
	if days == 0 && months == 0 {
		return fmt.Errorf("budget interval must be positive")
	}
	if days > MaxBudgetIntervalDays || months > MaxBudgetIntervalMonths {
		return fmt.Errorf("budget interval exceeds supported bounds")
	}
	return nil
}
