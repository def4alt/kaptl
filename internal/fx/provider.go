package fx

import (
	"context"
	"time"
)

// RateProvider resolves an auditable historical source-to-target quote.
// Implementations may call an external authority; callers must not invoke it
// while holding a ledger database transaction open.
type RateProvider interface {
	Name() string
	Quote(ctx context.Context, source, target string, at time.Time) (Quote, error)
}
