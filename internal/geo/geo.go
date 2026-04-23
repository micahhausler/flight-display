package geo

import "math"

const (
	earthRadiusM = 6_371_000.0 // mean earth radius in meters
	deg2rad      = math.Pi / 180.0
	rad2deg      = 180.0 / math.Pi
	ftToM        = 0.3048
)

// Bearing computes the initial bearing (azimuth) in degrees from point 1 to point 2.
// Result is in [0, 360).
func Bearing(lat1, lon1, lat2, lon2 float64) float64 {
	φ1 := lat1 * deg2rad
	φ2 := lat2 * deg2rad
	Δλ := (lon2 - lon1) * deg2rad

	y := math.Sin(Δλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(Δλ)
	θ := math.Atan2(y, x)

	return math.Mod(θ*rad2deg+360, 360)
}

// GroundDistanceM computes the great-circle distance in meters between two points.
func GroundDistanceM(lat1, lon1, lat2, lon2 float64) float64 {
	φ1 := lat1 * deg2rad
	φ2 := lat2 * deg2rad
	Δφ := (lat2 - lat1) * deg2rad
	Δλ := (lon2 - lon1) * deg2rad

	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusM * c
}

// Elevation computes the elevation angle in degrees from an observer at altObserverM
// (meters MSL) to a target at altTargetM (meters MSL), given the ground distance in meters.
// Positive means above horizon, negative means below.
func Elevation(groundDistM, altObserverM, altTargetM float64) float64 {
	if groundDistM == 0 {
		if altTargetM > altObserverM {
			return 90
		}
		return -90
	}
	altDiff := altTargetM - altObserverM
	return math.Atan2(altDiff, groundDistM) * rad2deg
}

// AltFtToM converts feet to meters.
func AltFtToM(ft float64) float64 {
	return ft * ftToM
}

// BBoxDeg computes a bounding box in degrees around a lat/lon point with a given
// radius in meters. Returns (latMin, lonMin, latMax, lonMax).
func BBoxDeg(lat, lon, radiusM float64) (float64, float64, float64, float64) {
	// Approximate: 1 degree latitude ≈ 111,320 m
	dLat := (radiusM / 111_320.0)
	// 1 degree longitude = 111,320 * cos(lat)
	dLon := radiusM / (111_320.0 * math.Cos(lat*deg2rad))

	return lat - dLat, lon - dLon, lat + dLat, lon + dLon
}
