package idle

import (
	"math"
	"testing"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

func TestSunriseSunsetSeattle(t *testing.T) {
	// Seattle on summer solstice 2024 — sunrise ~5:11am, sunset ~9:11pm PDT
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 6, 20, 12, 0, 0, 0, loc)

	rise, set, ok := sunriseSunset(47.6062, -122.3321, date)
	if !ok {
		t.Fatal("sunriseSunset returned !ok for Seattle summer solstice")
	}

	// Check sunrise is between 5:00 and 5:30 AM PDT
	riseHour := rise.Hour()
	if riseHour < 5 || riseHour > 5 {
		t.Errorf("sunrise hour = %d, want ~5 (got %s)", riseHour, rise.Format("3:04 PM"))
	}

	// Check sunset is between 9:00 and 9:30 PM PDT
	setHour := set.Hour()
	if setHour < 21 || setHour > 21 {
		t.Errorf("sunset hour = %d, want ~21 (got %s)", setHour, set.Format("3:04 PM"))
	}

	// Sanity: sunrise before sunset
	if !rise.Before(set) {
		t.Errorf("sunrise (%s) should be before sunset (%s)", rise, set)
	}
}

func TestSunriseSunsetWinterSolstice(t *testing.T) {
	// Seattle on winter solstice — sunrise ~7:55am, sunset ~4:20pm PST
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 12, 21, 12, 0, 0, 0, loc)

	rise, set, ok := sunriseSunset(47.6062, -122.3321, date)
	if !ok {
		t.Fatal("sunriseSunset returned !ok for Seattle winter solstice")
	}

	// Sunrise should be around 7:50-8:00 AM PST
	if rise.Hour() < 7 || rise.Hour() > 8 {
		t.Errorf("winter sunrise hour = %d, want 7-8 (got %s)", rise.Hour(), rise.Format("3:04 PM"))
	}

	// Sunset should be around 4:15-4:25 PM PST
	if set.Hour() < 16 || set.Hour() > 16 {
		t.Errorf("winter sunset hour = %d, want 16 (got %s)", set.Hour(), set.Format("3:04 PM"))
	}
}

func TestSunriseSunsetEquator(t *testing.T) {
	// Equator: sunrise and sunset should be close to 6:00 AM / 6:00 PM year-round
	date := time.Date(2024, 3, 20, 12, 0, 0, 0, time.UTC)

	rise, set, ok := sunriseSunset(0.0, 0.0, date)
	if !ok {
		t.Fatal("sunriseSunset returned !ok for equator")
	}

	// Day length at equator on equinox should be ~12 hours
	dayLength := set.Sub(rise).Hours()
	if math.Abs(dayLength-12.0) > 0.5 {
		t.Errorf("equator equinox day length = %.2f hours, want ~12", dayLength)
	}
}

func TestSunProviderNextEvent(t *testing.T) {
	// The SunProvider should always return data for non-polar locations
	p := NewSunProvider(47.6062, -122.3321) // Seattle
	info, ok := p.Current()
	if !ok {
		t.Fatal("SunProvider.Current() returned !ok for Seattle")
	}
	if info.Icon != model.IconSunrise && info.Icon != model.IconSunset {
		t.Errorf("Icon = %d, want IconSunrise (%d) or IconSunset (%d)", info.Icon, model.IconSunrise, model.IconSunset)
	}
	if info.Primary == "" {
		t.Error("Primary should not be empty")
	}
}

func TestSunriseSunsetPolarNoRise(t *testing.T) {
	// North pole in winter — sun never rises
	date := time.Date(2024, 12, 21, 12, 0, 0, 0, time.UTC)
	_, _, ok := sunriseSunset(89.0, 0.0, date)
	if ok {
		t.Error("expected !ok for polar winter (no sunrise)")
	}
}
