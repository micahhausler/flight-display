package tracker

import (
	"log"
	"math"
	"regexp"
	"time"

	"github.com/micahhausler/flight-display/internal/config"
	"github.com/micahhausler/flight-display/internal/geo"
	"github.com/micahhausler/flight-display/internal/model"
	"github.com/micahhausler/flight-display/internal/route"
)

const (
	altChangeThreshold     = 200.0 // feet
	bearingChangeThreshold = 5.0   // degrees
)

// RouteLookup provides route information for a callsign.
type RouteLookup interface {
	Lookup(callsign string) (*model.Route, error)
}

// airlineCallsignRe matches ICAO airline callsigns: 2-3 letter code followed by
// digits with optional letter suffix (e.g. DAL1714, SKW112J, ASA95).
// Rejects registration numbers like N724KP, CFUEI, G-ABCD.
var airlineCallsignRe = regexp.MustCompile(`^[A-Z]{2,3}\d+[A-Z]{0,2}$`)

// Tracker maintains the set of active sightings and emits events.
type Tracker struct {
	observer       config.Observer
	aperture       config.Aperture
	ttl            time.Duration
	maxRangeM      float64
	minAltFt       float64
	minSpeedKt     float64
	commercialOnly bool
	routeDB        *route.DB
	routeLookup    RouteLookup // optional: AeroAPI or other live route source

	active map[string]*model.Sighting // keyed by ICAO24
}

// New creates a new Tracker.
func New(obs config.Observer, ap config.Aperture, ttl time.Duration, maxRangeKM float64, routeDB *route.DB) *Tracker {
	return &Tracker{
		observer:  obs,
		aperture:  ap,
		ttl:       ttl,
		maxRangeM: maxRangeKM * 1000,
		routeDB:   routeDB,
		active:    make(map[string]*model.Sighting),
	}
}

// SetRouteLookup sets an optional live route lookup source (e.g., AeroAPI).
// When set, this is used as the primary route source; VRS is the fallback.
func (t *Tracker) SetRouteLookup(rl RouteLookup) {
	t.routeLookup = rl
}

// SetFilters configures minimum altitude, speed, and commercial-only filters.
func (t *Tracker) SetFilters(minAltFt, minSpeedKt float64, commercialOnly bool) {
	t.minAltFt = minAltFt
	t.minSpeedKt = minSpeedKt
	t.commercialOnly = commercialOnly
}

// Process takes a batch of aircraft from a poll and returns events.
func (t *Tracker) Process(aircraft []model.Aircraft) []model.Event {
	var events []model.Event
	seen := make(map[string]bool)

	for _, ac := range aircraft {
		if !ac.HasPosition() || ac.OnGround {
			continue
		}
		if t.minAltFt > 0 && (ac.AltFt == nil || *ac.AltFt < t.minAltFt) {
			continue
		}
		if t.minSpeedKt > 0 && (ac.VelocityKt == nil || *ac.VelocityKt < t.minSpeedKt) {
			continue
		}
		if t.commercialOnly && (ac.Callsign == nil || !airlineCallsignRe.MatchString(*ac.Callsign)) {
			continue
		}

		bearing, elevation, slantRangeM := t.computeBearingElevation(ac)
		if !t.inAperture(bearing, elevation) {
			continue
		}
		if t.maxRangeM > 0 && slantRangeM > t.maxRangeM {
			continue
		}

		seen[ac.ICAO24] = true

		if existing, ok := t.active[ac.ICAO24]; ok {
			// Update existing sighting
			existing.LastPosition = ac.TimePosition
			existing.Aircraft = ac

			if t.materialChange(existing, bearing, elevation) {
				existing.BearingDeg = bearing
				existing.ElevationDeg = elevation
				events = append(events, model.Event{
					Kind:     model.Update,
					Sighting: *existing,
				})
			}
		} else {
			// New sighting
			var r *model.Route
			if ac.Callsign != nil {
				r = t.lookupRoute(*ac.Callsign)
			}

			sighting := &model.Sighting{
				Aircraft:     ac,
				Route:        r,
				BearingDeg:   bearing,
				ElevationDeg: elevation,
				FirstSeen:    time.Now(),
				LastPosition: ac.TimePosition,
			}
			t.active[ac.ICAO24] = sighting
			events = append(events, model.Event{
				Kind:     model.Enter,
				Sighting: *sighting,
			})
		}
	}

	// Check for expired sightings
	now := time.Now()
	for icao, sighting := range t.active {
		if seen[icao] {
			continue
		}
		if now.Sub(sighting.LastPosition) >= t.ttl {
			events = append(events, model.Event{
				Kind:     model.Leave,
				Sighting: *sighting,
			})
			delete(t.active, icao)
		}
	}

	return events
}

// ActiveCount returns the number of currently tracked sightings.
func (t *Tracker) ActiveCount() int {
	return len(t.active)
}

func (t *Tracker) computeBearingElevation(ac model.Aircraft) (float64, float64, float64) {
	bearing := geo.Bearing(t.observer.Lat, t.observer.Lon, *ac.Lat, *ac.Lon)
	groundDist := geo.GroundDistanceM(t.observer.Lat, t.observer.Lon, *ac.Lat, *ac.Lon)

	acAltM := 0.0
	if ac.AltFt != nil {
		acAltM = geo.AltFtToM(*ac.AltFt)
	}
	elevation := geo.Elevation(groundDist, t.observer.AltMSLMeter, acAltM)

	altDiff := acAltM - t.observer.AltMSLMeter
	slantRange := math.Sqrt(groundDist*groundDist + altDiff*altDiff)

	return bearing, elevation, slantRange
}

func (t *Tracker) inAperture(azimuth, elevation float64) bool {
	for _, r := range t.aperture.Rects {
		if inAzRange(azimuth, r.AzMin, r.AzMax) && elevation >= r.ElMin && elevation <= r.ElMax {
			return true
		}
	}
	return false
}

// inAzRange checks if azimuth is within [min, max], handling wrap-around.
func inAzRange(az, min, max float64) bool {
	if min <= max {
		return az >= min && az <= max
	}
	// Wraps around 360: e.g., min=350, max=10
	return az >= min || az <= max
}

func (t *Tracker) materialChange(s *model.Sighting, newBearing, newElevation float64) bool {
	if s.Aircraft.AltFt != nil {
		oldAlt := 0.0
		if s.Aircraft.AltFt != nil {
			oldAlt = *s.Aircraft.AltFt
		}
		newAlt := 0.0
		if s.Aircraft.AltFt != nil {
			newAlt = *s.Aircraft.AltFt
		}
		if math.Abs(newAlt-oldAlt) > altChangeThreshold {
			return true
		}
	}
	if math.Abs(newBearing-s.BearingDeg) > bearingChangeThreshold {
		return true
	}
	return false
}

// lookupRoute tries the live route source first (AeroAPI), falls back to VRS.
func (t *Tracker) lookupRoute(callsign string) *model.Route {
	if t.routeLookup != nil {
		r, err := t.routeLookup.Lookup(callsign)
		if err != nil {
			log.Printf("Route lookup error for %s: %v (falling back to VRS)", callsign, err)
			return t.routeDB.Lookup(callsign)
		}
		return r
	}
	return t.routeDB.Lookup(callsign)
}
