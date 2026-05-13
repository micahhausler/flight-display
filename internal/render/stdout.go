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
	switch event.Kind {
	case model.Idle:
		icon := idleIconStr(event.IdleInfo.Icon)
		fmt.Printf("  %s %s\n", icon, event.IdleInfo.Primary)
		return

	case model.Enter:
		// Suppress aircraft with no callsign
		if event.Sighting.Aircraft.Callsign == nil {
			return
		}
		callsign := sightingCallsign(event.Sighting)
		routeStr := displayRoute(event.Sighting.Route)
		alt := ""
		if event.Sighting.Aircraft.AltFt != nil {
			alt = fmt.Sprintf("%dft", int(*event.Sighting.Aircraft.AltFt))
		}
		// Fixed columns: callsign(8) + route(12) + alt(7, right-aligned)
		fmt.Printf("+ %-8s %-12s %7s\n", callsign, routeStr, alt)

	case model.Leave:
		// Suppress aircraft with no callsign
		if event.Sighting.Aircraft.Callsign == nil {
			return
		}
		callsign := sightingCallsign(event.Sighting)
		routeStr := displayRoute(event.Sighting.Route)
		fmt.Printf("- %-8s %s\n", callsign, routeStr)

	case model.Update:
		// V1: suppress update events
	}
}

// sightingCallsign returns the display callsign, preferring the marketing carrier
// ident (codeshare resolution) when available.
func sightingCallsign(s model.Sighting) string {
	if s.DisplayCallsign != nil {
		return *s.DisplayCallsign
	}
	return *s.Aircraft.Callsign
}

// idleIconStr maps an IdleIcon to its emoji representation for stdout.
func idleIconStr(icon model.IdleIcon) string {
	switch icon {
	case model.IconClock:
		return "\U0001f559" // 🕙
	case model.IconDate:
		return "\U0001f4c5" // 📅
	case model.IconSunrise:
		return "\U0001f305" // 🌅
	case model.IconSunset:
		return "\U0001f305" // 🌅
	case model.IconTemperature:
		return "\U0001f321\ufe0f" // 🌡️
	default:
		return " "
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
