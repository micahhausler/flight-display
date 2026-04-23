package opensky

import (
	"strings"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

const (
	metersToFeet = 3.28084
	msToKnots    = 1.94384
)

// Normalize converts an OpenSky StateResponse into a slice of Aircraft.
// Unit conversions (meters→feet, m/s→knots) happen here and nowhere else.
func Normalize(resp *StateResponse) []model.Aircraft {
	if resp == nil || len(resp.States) == 0 {
		return nil
	}

	result := make([]model.Aircraft, 0, len(resp.States))
	for _, sv := range resp.States {
		a, ok := normalizeOne(sv)
		if !ok {
			continue
		}
		result = append(result, a)
	}
	return result
}

func normalizeOne(sv []interface{}) (model.Aircraft, bool) {
	if len(sv) < 17 {
		return model.Aircraft{}, false
	}

	icao24, ok := sv[0].(string)
	if !ok || icao24 == "" {
		return model.Aircraft{}, false
	}

	a := model.Aircraft{
		ICAO24: strings.ToLower(icao24),
	}

	// Callsign (index 1)
	if cs, ok := sv[1].(string); ok {
		cs = strings.TrimSpace(cs)
		if cs != "" {
			a.Callsign = &cs
		}
	}

	// Longitude (index 5)
	if v, ok := toFloat(sv[5]); ok {
		a.Lon = &v
	}
	// Latitude (index 6)
	if v, ok := toFloat(sv[6]); ok {
		a.Lat = &v
	}

	// Barometric altitude in meters (index 7), fallback to geometric (index 13)
	if v, ok := toFloat(sv[7]); ok {
		ft := v * metersToFeet
		a.AltFt = &ft
	} else if v, ok := toFloat(sv[13]); ok {
		ft := v * metersToFeet
		a.AltFt = &ft
	}

	// On ground (index 8)
	if v, ok := sv[8].(bool); ok {
		a.OnGround = v
	}

	// Velocity m/s (index 9)
	if v, ok := toFloat(sv[9]); ok {
		kt := v * msToKnots
		a.VelocityKt = &kt
	}

	// True track (index 10)
	if v, ok := toFloat(sv[10]); ok {
		a.HeadingDeg = &v
	}

	// Time position (index 3) — source timestamp, not ingestion time
	if v, ok := toFloat(sv[3]); ok {
		a.TimePosition = time.Unix(int64(v), 0)
	} else {
		// Fallback to last_contact (index 4)
		if v, ok := toFloat(sv[4]); ok {
			a.TimePosition = time.Unix(int64(v), 0)
		} else {
			a.TimePosition = time.Now()
		}
	}

	return a, true
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
