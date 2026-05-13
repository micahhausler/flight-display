// Package idle provides ambient information providers and a rotator for
// display when no aircraft are in the view window.
package idle

import "github.com/micahhausler/flight-display/internal/model"

// Provider supplies one kind of ambient idle information.
type Provider interface {
	// Name returns the provider's identifier (e.g., "clock", "weather").
	Name() string
	// Current returns the current idle info to display.
	// Returns false if the provider has no data available (e.g., weather fetch failed).
	Current() (model.IdleInfo, bool)
}

// Rotator cycles through providers in round-robin order.
type Rotator struct {
	providers []Provider
	idx       int
}

// NewRotator creates a rotator with the given providers.
func NewRotator(providers []Provider) *Rotator {
	return &Rotator{
		providers: providers,
	}
}

// Next advances to the next provider and returns its current info as an Event.
// Returns false if no provider has data available (all skipped).
func (r *Rotator) Next() (model.Event, bool) {
	if len(r.providers) == 0 {
		return model.Event{}, false
	}

	// Try each provider at most once to find one with data.
	for range r.providers {
		r.idx = (r.idx + 1) % len(r.providers)
		info, ok := r.providers[r.idx].Current()
		if ok {
			return model.Event{
				Kind:     model.Idle,
				IdleInfo: info,
			}, true
		}
	}
	return model.Event{}, false
}

// Reset returns the rotator to the beginning (first provider will be next).
func (r *Rotator) Reset() {
	// Set to len-1 so that Next() wraps to index 0.
	if len(r.providers) > 0 {
		r.idx = len(r.providers) - 1
	}
}
