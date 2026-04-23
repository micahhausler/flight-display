package render

import (
	"fmt"

	"github.com/micahhausler/flight-display/internal/model"
)

// Renderer consumes events and writes output.
type Renderer interface {
	Render(event model.Event)
}

// Stdout renders flight events to standard output.
type Stdout struct{}

// NewStdout creates a new STDOUT renderer.
func NewStdout() *Stdout {
	return &Stdout{}
}

func (s *Stdout) Render(event model.Event) {
	// Suppress aircraft with no callsign
	if event.Sighting.Aircraft.Callsign == nil {
		return
	}

	callsign := *event.Sighting.Aircraft.Callsign
	routeStr := displayRoute(event.Sighting.Route)

	switch event.Kind {
	case model.Enter:
		alt := ""
		if event.Sighting.Aircraft.AltFt != nil {
			alt = fmt.Sprintf("%dft", int(*event.Sighting.Aircraft.AltFt))
		}
		// Fixed columns: callsign(8) + route(12) + alt(7, right-aligned)
		fmt.Printf("+ %-8s %-12s %7s\n", callsign, routeStr, alt)

	case model.Leave:
		fmt.Printf("- %-8s %s\n", callsign, routeStr)

	case model.Update:
		// V1: suppress update events
	}
}

// displayRoute returns the last leg of a route for display (e.g., "SEA-BNA").
// Multi-leg routes like SEA-RDM-SEA are trimmed to the final segment (RDM-SEA).
func displayRoute(r *model.Route) string {
	if r == nil || len(r.Airports) < 2 {
		return ""
	}
	n := len(r.Airports)
	return r.Airports[n-2] + "-" + r.Airports[n-1]
}
