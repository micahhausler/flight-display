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
	routeStr := event.Sighting.Route.String()

	switch event.Kind {
	case model.Enter:
		alt := ""
		if event.Sighting.Aircraft.AltFt != nil {
			alt = fmt.Sprintf("%dft", int(*event.Sighting.Aircraft.AltFt))
		}
		if routeStr != "" {
			fmt.Printf("+ %-8s %-7s %7s\n", callsign, routeStr, alt)
		} else {
			fmt.Printf("+ %-8s         %7s\n", callsign, alt)
		}

	case model.Leave:
		if routeStr != "" {
			fmt.Printf("- %-8s %s\n", callsign, routeStr)
		} else {
			fmt.Printf("- %s\n", callsign)
		}

	case model.Update:
		// V1: suppress update events
	}
}
