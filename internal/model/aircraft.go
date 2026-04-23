package model

import "time"

// Aircraft is a normalized representation of one aircraft at a moment in time.
// Pointer fields distinguish "unknown" from zero.
type Aircraft struct {
	ICAO24       string   // unique transponder address, hex (e.g. "a1b2c3")
	Callsign     *string  // e.g. "DAL1921". Nil when not transmitted.
	Lat          *float64 // WGS-84 decimal degrees
	Lon          *float64 // WGS-84 decimal degrees
	AltFt        *float64 // barometric altitude in feet
	HeadingDeg   *float64 // true track, degrees clockwise from north
	VelocityKt   *float64 // ground speed in knots
	OnGround     bool
	TimePosition time.Time // source-reported position timestamp
}

// HasPosition returns true if lat, lon, and altitude are all known.
func (a Aircraft) HasPosition() bool {
	return a.Lat != nil && a.Lon != nil
}
