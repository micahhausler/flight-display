package model

import "time"

// Route holds the airports for a flight's route.
type Route struct {
	Airports []string // IATA codes in order flown, e.g. ["SEA", "BNA"]
}

// FormatRoute returns a hyphen-separated string like "SEA-BNA", or empty string if nil.
func (r *Route) String() string {
	if r == nil || len(r.Airports) == 0 {
		return ""
	}
	s := r.Airports[0]
	for _, a := range r.Airports[1:] {
		s += "-" + a
	}
	return s
}

// Sighting represents a flight that has entered the aperture and may still be in it.
type Sighting struct {
	Aircraft     Aircraft
	Route        *Route
	BearingDeg   float64   // azimuth from observer
	ElevationDeg float64   // elevation from observer
	FirstSeen    time.Time // when this aircraft first entered the aperture
	LastPosition time.Time // most recent source-reported time_position while visible
}

// EventKind indicates whether a flight entered, updated, or left the aperture.
type EventKind int

const (
	Enter  EventKind = iota // aircraft entered the aperture
	Update                  // aircraft still in aperture, state changed materially
	Leave                   // aircraft left the aperture or TTL expired
)

// Event represents a change in the set of visible aircraft.
type Event struct {
	Kind     EventKind
	Sighting Sighting
}
