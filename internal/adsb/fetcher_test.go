package adsb

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFetchFromHTTP(t *testing.T) {
	fixture, err := os.ReadFile("testdata/aircraft.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL)
	aircraft, err := f.Fetch()
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if len(aircraft) != 5 {
		t.Fatalf("expected 5 aircraft, got %d", len(aircraft))
	}

	// Verify ASA375 — full ADS-B with all fields
	asa := aircraft[0]
	if asa.ICAO24 != "a2ce05" {
		t.Errorf("aircraft[0].ICAO24 = %q, want %q", asa.ICAO24, "a2ce05")
	}
	if asa.Callsign == nil || *asa.Callsign != "ASA375" {
		t.Errorf("aircraft[0].Callsign = %v, want ASA375", asa.Callsign)
	}
	if asa.Lat == nil || math.Abs(*asa.Lat-47.523056) > 0.0001 {
		t.Errorf("aircraft[0].Lat = %v, want ~47.523056", asa.Lat)
	}
	if asa.Lon == nil || math.Abs(*asa.Lon-(-122.317314)) > 0.0001 {
		t.Errorf("aircraft[0].Lon = %v, want ~-122.317314", asa.Lon)
	}
	// alt_baro is already in feet from readsb — no conversion
	if asa.AltFt == nil || *asa.AltFt != 1575 {
		t.Errorf("aircraft[0].AltFt = %v, want 1575", asa.AltFt)
	}
	// gs is already in knots from readsb
	if asa.VelocityKt == nil || *asa.VelocityKt != 144.0 {
		t.Errorf("aircraft[0].VelocityKt = %v, want 144.0", asa.VelocityKt)
	}
	if asa.HeadingDeg == nil || math.Abs(*asa.HeadingDeg-180.40) > 0.01 {
		t.Errorf("aircraft[0].HeadingDeg = %v, want ~180.40", asa.HeadingDeg)
	}
	if asa.OnGround {
		t.Errorf("aircraft[0].OnGround = true, want false")
	}

	// Verify UAL2101
	ual := aircraft[1]
	if ual.ICAO24 != "a2b17a" {
		t.Errorf("aircraft[1].ICAO24 = %q, want %q", ual.ICAO24, "a2b17a")
	}
	if ual.Callsign == nil || *ual.Callsign != "UAL2101" {
		t.Errorf("aircraft[1].Callsign = %v, want UAL2101", ual.Callsign)
	}
	if ual.AltFt == nil || *ual.AltFt != 2875 {
		t.Errorf("aircraft[1].AltFt = %v, want 2875", ual.AltFt)
	}

	// Verify mode_s aircraft (no callsign, no position)
	modeS := aircraft[3]
	if modeS.ICAO24 != "a45316" {
		t.Errorf("aircraft[3].ICAO24 = %q, want %q", modeS.ICAO24, "a45316")
	}
	if modeS.Callsign != nil {
		t.Errorf("aircraft[3].Callsign = %v, want nil", modeS.Callsign)
	}
	if modeS.Lat != nil {
		t.Errorf("aircraft[3].Lat = %v, want nil (mode_s has no position)", modeS.Lat)
	}
	if modeS.AltFt == nil || *modeS.AltFt != 1725 {
		t.Errorf("aircraft[3].AltFt = %v, want 1725", modeS.AltFt)
	}

	// Verify ground aircraft (alt_baro: "ground")
	ground := aircraft[4]
	if ground.ICAO24 != "c07a03" {
		t.Errorf("aircraft[4].ICAO24 = %q, want %q", ground.ICAO24, "c07a03")
	}
	if !ground.OnGround {
		t.Errorf("aircraft[4].OnGround = false, want true (alt_baro was \"ground\")")
	}
	if ground.AltFt != nil {
		t.Errorf("aircraft[4].AltFt = %v, want nil (on ground)", ground.AltFt)
	}
	if ground.Callsign == nil || *ground.Callsign != "ACA101" {
		t.Errorf("aircraft[4].Callsign = %v, want ACA101", ground.Callsign)
	}

	// Verify TimePosition is computed from "now" minus "seen"
	// now=1778696187, ASA375 seen=0.1
	expectedTime := time.Unix(1778696187, 0).Add(-100 * time.Millisecond)
	if math.Abs(float64(asa.TimePosition.Sub(expectedTime))) > float64(50*time.Millisecond) {
		t.Errorf("aircraft[0].TimePosition = %v, want ~%v", asa.TimePosition, expectedTime)
	}
}

func TestMinInterval(t *testing.T) {
	f := NewFetcher("http://localhost:9999")
	if got := f.MinInterval(); got != 5*time.Second {
		t.Errorf("MinInterval() = %v, want 5s", got)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewFetcher(srv.URL)
	_, err := f.Fetch()
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestFetchConnectionRefused(t *testing.T) {
	f := NewFetcher("http://localhost:1") // nothing listening
	_, err := f.Fetch()
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}
