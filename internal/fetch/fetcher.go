package fetch

import (
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

// Fetcher is the contract between a data source and the tracker.
// Each implementation handles its own bounding-box filtering (if applicable)
// and rate-limit semantics. The poll loop uses MinInterval() as the floor
// for polling frequency.
type Fetcher interface {
	// Fetch returns the current set of aircraft from the source.
	// Returns nil, nil to signal a skipped poll (e.g., rate-limited).
	Fetch() ([]model.Aircraft, error)

	// MinInterval returns the minimum polling interval this source supports.
	// The poll loop will not poll faster than this, regardless of configuration.
	MinInterval() time.Duration
}
