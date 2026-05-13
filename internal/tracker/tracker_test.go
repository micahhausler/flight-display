package tracker

import (
	"testing"
	"time"

	"github.com/micahhausler/flight-display/internal/config"
	"github.com/micahhausler/flight-display/internal/model"
)

func ptr(f float64) *float64 { return &f }
func strp(s string) *string  { return &s }

func TestProcess_EnterAndLeave(t *testing.T) {
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 160, AzMax: 290, ElMin: -2, ElMax: 30},
		},
	}
	trk := New(obs, ap, 2*time.Second, 0, nil) // 0 = no range limit

	// Aircraft to the south-southwest at ~35,000ft, ~100km away
	aircraft := []model.Aircraft{
		{
			ICAO24:       "a1b2c3",
			Callsign:     strp("DAL1921"),
			Lat:          ptr(46.8),   // south of observer
			Lon:          ptr(-122.5), // slightly west
			AltFt:        ptr(35000.0),
			HeadingDeg:   ptr(180.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(aircraft)

	// Should get an Enter event
	enterCount := 0
	for _, e := range events {
		if e.Kind == model.Enter {
			enterCount++
			if e.Sighting.Aircraft.ICAO24 != "a1b2c3" {
				t.Errorf("expected ICAO24 a1b2c3, got %s", e.Sighting.Aircraft.ICAO24)
			}
		}
	}
	if enterCount != 1 {
		t.Errorf("expected 1 Enter event, got %d", enterCount)
	}
	if trk.ActiveCount() != 1 {
		t.Errorf("expected 1 active sighting, got %d", trk.ActiveCount())
	}

	// Process again with same aircraft — should get no Enter
	events = trk.Process(aircraft)
	for _, e := range events {
		if e.Kind == model.Enter {
			t.Error("unexpected Enter event on second poll")
		}
	}

	// Process with empty list, wait for TTL
	time.Sleep(3 * time.Second)
	events = trk.Process(nil)

	leaveCount := 0
	for _, e := range events {
		if e.Kind == model.Leave {
			leaveCount++
		}
	}
	if leaveCount != 1 {
		t.Errorf("expected 1 Leave event, got %d", leaveCount)
	}
	if trk.ActiveCount() != 0 {
		t.Errorf("expected 0 active sightings, got %d", trk.ActiveCount())
	}
}

func TestProcess_NotInAperture(t *testing.T) {
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 160, AzMax: 290, ElMin: -2, ElMax: 30},
		},
	}
	trk := New(obs, ap, 60*time.Second, 0, nil) // 0 = no range limit

	// Aircraft to the north — outside the south-through-west aperture
	aircraft := []model.Aircraft{
		{
			ICAO24:       "d4e5f6",
			Callsign:     strp("UAL123"),
			Lat:          ptr(48.5), // north of observer
			Lon:          ptr(-122.3),
			AltFt:        ptr(30000.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(aircraft)
	for _, e := range events {
		if e.Kind == model.Enter {
			t.Error("aircraft to the north should not enter south-facing aperture")
		}
	}
	if trk.ActiveCount() != 0 {
		t.Errorf("expected 0 active sightings, got %d", trk.ActiveCount())
	}
}

func TestProcess_OnGroundSuppressed(t *testing.T) {
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 0, AzMax: 360, ElMin: -90, ElMax: 90}, // full sky
		},
	}
	trk := New(obs, ap, 60*time.Second, 0, nil) // 0 = no range limit

	aircraft := []model.Aircraft{
		{
			ICAO24:       "aabbcc",
			Callsign:     strp("ASA100"),
			Lat:          ptr(47.45),
			Lon:          ptr(-122.31),
			AltFt:        ptr(0.0),
			OnGround:     true,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(aircraft)
	if len(events) != 0 {
		t.Errorf("on-ground aircraft should not produce events, got %d", len(events))
	}
}

func TestProcess_BeyondMaxRange(t *testing.T) {
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 160, AzMax: 290, ElMin: -2, ElMax: 30},
		},
	}
	// 50km max range
	trk := New(obs, ap, 60*time.Second, 50, nil)

	// Aircraft in aperture but ~90km away (46.8°N is ~90km south of 47.6°N)
	farAircraft := []model.Aircraft{
		{
			ICAO24:       "far123",
			Callsign:     strp("DAL999"),
			Lat:          ptr(46.8),
			Lon:          ptr(-122.5),
			AltFt:        ptr(35000.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(farAircraft)
	for _, e := range events {
		if e.Kind == model.Enter {
			t.Error("aircraft 90km away should be rejected with 50km max range")
		}
	}

	// Aircraft in aperture and ~20km away — should be accepted
	nearAircraft := []model.Aircraft{
		{
			ICAO24:       "near456",
			Callsign:     strp("ASA100"),
			Lat:          ptr(47.45),
			Lon:          ptr(-122.4),
			AltFt:        ptr(10000.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events = trk.Process(nearAircraft)
	enterCount := 0
	for _, e := range events {
		if e.Kind == model.Enter {
			enterCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("aircraft 20km away should be accepted with 50km max range, got %d enters", enterCount)
	}
}

// mockRouteLookup implements RouteLookup for testing.
type mockRouteLookup struct {
	info map[string]*model.FlightInfo
}

func (m *mockRouteLookup) Lookup(callsign string) (*model.FlightInfo, error) {
	if info, ok := m.info[callsign]; ok {
		return info, nil
	}
	return nil, nil
}

func TestProcess_DisplayCallsignFromRouteLookup(t *testing.T) {
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 0, AzMax: 360, ElMin: -90, ElMax: 90}, // full sky
		},
	}
	trk := New(obs, ap, 60*time.Second, 0, nil)

	// Simulate AeroAPI resolving a codeshare: SKW5123 -> UAL1234
	trk.SetRouteLookup(&mockRouteLookup{
		info: map[string]*model.FlightInfo{
			"SKW5123": {
				Route:           &model.Route{Airports: []string{"SEA", "SFO"}},
				DisplayCallsign: strp("UAL1234"),
			},
		},
	})

	aircraft := []model.Aircraft{
		{
			ICAO24:       "abc123",
			Callsign:     strp("SKW5123"),
			Lat:          ptr(47.45),
			Lon:          ptr(-122.4),
			AltFt:        ptr(10000.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(aircraft)
	if len(events) != 1 || events[0].Kind != model.Enter {
		t.Fatalf("expected 1 Enter event, got %d events", len(events))
	}

	sighting := events[0].Sighting
	if sighting.DisplayCallsign == nil {
		t.Fatal("expected DisplayCallsign to be set")
	}
	if *sighting.DisplayCallsign != "UAL1234" {
		t.Errorf("DisplayCallsign = %q, want UAL1234", *sighting.DisplayCallsign)
	}
	if sighting.Route == nil || sighting.Route.Airports[1] != "SFO" {
		t.Errorf("Route = %v, want SEA-SFO", sighting.Route)
	}
	// Raw callsign is preserved
	if *sighting.Aircraft.Callsign != "SKW5123" {
		t.Errorf("Aircraft.Callsign = %q, want SKW5123 (raw preserved)", *sighting.Aircraft.Callsign)
	}
}

func TestProcess_CommercialOnlyFiltersOnRawCallsign(t *testing.T) {
	// Verifies that commercial_only filter operates on the raw transponder callsign,
	// not the display callsign. A SkyWest callsign (SKW5123) is a valid airline
	// callsign and should pass the filter even though it's a regional operator.
	obs := config.Observer{Lat: 47.6115, Lon: -122.347, AltMSLMeter: 55}
	ap := config.Aperture{
		Rects: []config.AzElRect{
			{AzMin: 0, AzMax: 360, ElMin: -90, ElMax: 90}, // full sky
		},
	}
	trk := New(obs, ap, 60*time.Second, 0, nil)
	trk.SetFilters(0, 0, true) // commercial_only = true

	trk.SetRouteLookup(&mockRouteLookup{
		info: map[string]*model.FlightInfo{
			"SKW5123": {
				Route:           &model.Route{Airports: []string{"SEA", "SFO"}},
				DisplayCallsign: strp("UAL1234"),
			},
		},
	})

	aircraft := []model.Aircraft{
		{
			ICAO24:       "abc123",
			Callsign:     strp("SKW5123"), // valid airline callsign — passes filter
			Lat:          ptr(47.45),
			Lon:          ptr(-122.4),
			AltFt:        ptr(10000.0),
			VelocityKt:   ptr(250.0),
			OnGround:     false,
			TimePosition: time.Now(),
		},
	}

	events := trk.Process(aircraft)
	enterCount := 0
	for _, e := range events {
		if e.Kind == model.Enter {
			enterCount++
		}
	}
	if enterCount != 1 {
		t.Errorf("SKW5123 should pass commercial_only filter (valid airline callsign), got %d enters", enterCount)
	}
}
