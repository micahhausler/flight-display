package idle

import (
	"fmt"
	"math"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

// SunProvider computes the next sunrise or sunset for the observer's location
// using the NOAA simplified solar position algorithm.
type SunProvider struct {
	lat float64
	lon float64
}

func NewSunProvider(lat, lon float64) *SunProvider {
	return &SunProvider{lat: lat, lon: lon}
}

func (p *SunProvider) Name() string { return "sunrise_sunset" }

func (p *SunProvider) Current() (model.IdleInfo, bool) {
	now := time.Now()
	rise, set, ok := sunriseSunset(p.lat, p.lon, now)
	if !ok {
		// Polar day/night — no sunrise or sunset today
		return model.IdleInfo{}, false
	}

	// Determine which event is next
	var icon model.IdleIcon
	var nextTime time.Time
	var arrow string

	if now.Before(rise) {
		// Before sunrise today
		icon = model.IconSunrise
		nextTime = rise
		arrow = "🔼"
	} else if now.Before(set) {
		// After sunrise, before sunset
		icon = model.IconSunset
		nextTime = set
		arrow = "🔽"
	} else {
		// After sunset — show tomorrow's sunrise
		tomorrow := now.AddDate(0, 0, 1)
		rise2, _, ok2 := sunriseSunset(p.lat, p.lon, tomorrow)
		if !ok2 {
			return model.IdleInfo{}, false
		}
		icon = model.IconSunrise
		nextTime = rise2
		arrow = "🔼"
	}

	return model.IdleInfo{
		Icon:    icon,
		Primary: fmt.Sprintf("🌅%s @ %s", arrow, nextTime.Format("3:04pm")),
	}, true
}

// sunriseSunset computes sunrise and sunset times for the given date and location.
// Returns false if the sun never rises or never sets (polar regions).
// Uses the NOAA solar calculator simplified algorithm.
func sunriseSunset(lat, lon float64, date time.Time) (sunrise, sunset time.Time, ok bool) {
	// Julian day number
	y := date.Year()
	m := int(date.Month())
	d := date.Day()

	if m <= 2 {
		y--
		m += 12
	}
	A := y / 100
	B := 2 - A + A/4
	jd := float64(int(365.25*float64(y+4716))) + float64(int(30.6001*float64(m+1))) + float64(d) + float64(B) - 1524.5

	// Julian century from J2000.0
	T := (jd - 2451545.0) / 36525.0

	// Sun's geometric mean longitude (degrees)
	L0 := math.Mod(280.46646+T*(36000.76983+0.0003032*T), 360.0)

	// Sun's mean anomaly (degrees)
	M := math.Mod(357.52911+T*(35999.05029-0.0001537*T), 360.0)
	Mrad := M * math.Pi / 180.0

	// Equation of center
	C := (1.914602-T*(0.004817+0.000014*T))*math.Sin(Mrad) +
		(0.019993-0.000101*T)*math.Sin(2*Mrad) +
		0.000289*math.Sin(3*Mrad)

	// Sun's true longitude
	sunLon := L0 + C

	// Sun's apparent longitude
	omega := 125.04 - 1934.136*T
	lambda := sunLon - 0.00569 - 0.00478*math.Sin(omega*math.Pi/180.0)
	lambdaRad := lambda * math.Pi / 180.0

	// Obliquity of the ecliptic
	epsilon0 := 23.0 + (26.0+(21.448-T*(46.815+T*(0.00059-T*0.001813)))/60.0)/60.0
	epsilon := epsilon0 + 0.00256*math.Cos(omega*math.Pi/180.0)
	epsilonRad := epsilon * math.Pi / 180.0

	// Sun's declination
	sinDecl := math.Sin(epsilonRad) * math.Sin(lambdaRad)
	decl := math.Asin(sinDecl)

	// Equation of time (minutes)
	y2 := math.Tan(epsilonRad/2.0) * math.Tan(epsilonRad/2.0)
	L0rad := L0 * math.Pi / 180.0
	eqTime := 4.0 * (180.0 / math.Pi) * (y2*math.Sin(2*L0rad) -
		2.0*0.016708634*math.Sin(Mrad) +
		4.0*0.016708634*y2*math.Sin(Mrad)*math.Cos(2*L0rad) -
		0.5*y2*y2*math.Sin(4*L0rad) -
		1.25*0.016708634*0.016708634*math.Sin(2*Mrad))

	// Hour angle for sunrise/sunset (solar zenith = 90.833 degrees)
	latRad := lat * math.Pi / 180.0
	zenith := 90.833 * math.Pi / 180.0
	cosHA := (math.Cos(zenith) - math.Sin(latRad)*math.Sin(decl)) / (math.Cos(latRad) * math.Cos(decl))

	if cosHA > 1.0 {
		// Sun never rises
		return time.Time{}, time.Time{}, false
	}
	if cosHA < -1.0 {
		// Sun never sets
		return time.Time{}, time.Time{}, false
	}

	HA := math.Acos(cosHA) * 180.0 / math.Pi

	// Solar noon (minutes from midnight UTC)
	solarNoon := 720.0 - 4.0*lon - eqTime

	// Sunrise and sunset in minutes from midnight UTC
	riseMin := solarNoon - HA*4.0
	setMin := solarNoon + HA*4.0

	// Convert to local time
	loc := date.Location()
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	sunrise = startOfDay.Add(time.Duration(riseMin * float64(time.Minute))).In(loc)
	sunset = startOfDay.Add(time.Duration(setMin * float64(time.Minute))).In(loc)

	return sunrise, sunset, true
}
