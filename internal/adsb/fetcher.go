package adsb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

// response is the top-level JSON structure from readsb's API.
type response struct {
	Now      float64    `json:"now"`
	Aircraft []aircraft `json:"aircraft"`
}

// aircraft is a single aircraft entry from readsb.
// readsb outputs altitude in feet and speed in knots — no unit conversion needed.
type aircraft struct {
	Hex     string      `json:"hex"`
	Flight  string      `json:"flight"`
	Lat     *float64    `json:"lat"`
	Lon     *float64    `json:"lon"`
	AltBaro jsonAltBaro `json:"alt_baro"` // can be number (feet) or string "ground"
	AltGeom *float64    `json:"alt_geom"` // geometric altitude in feet
	GS      *float64    `json:"gs"`       // ground speed in knots
	Track   *float64    `json:"track"`    // true track in degrees
	Seen    float64     `json:"seen"`     // seconds since last message
}

// jsonAltBaro handles the alt_baro field which can be a number or the string "ground".
type jsonAltBaro struct {
	Value    *float64
	OnGround bool
}

func (j *jsonAltBaro) UnmarshalJSON(data []byte) error {
	// Try string first ("ground")
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "ground" {
			j.OnGround = true
		}
		return nil
	}
	// Try number
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		j.Value = &f
		return nil
	}
	// null or unrecognized — leave as nil
	return nil
}

// Fetcher reads aircraft data from readsb's HTTP API.
type Fetcher struct {
	url        string
	httpClient *http.Client
}

// NewFetcher creates an ADS-B fetcher that queries the given readsb API URL.
func NewFetcher(url string) *Fetcher {
	return &Fetcher{
		url:        url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Fetch queries the readsb HTTP API and returns the current set of aircraft.
func (f *Fetcher) Fetch() ([]model.Aircraft, error) {
	resp, err := f.httpClient.Get(f.url)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", f.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("readsb API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var data response
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	now := time.Unix(int64(data.Now), int64((data.Now-float64(int64(data.Now)))*1e9))
	result := make([]model.Aircraft, 0, len(data.Aircraft))
	for _, ac := range data.Aircraft {
		a, ok := normalize(ac, now)
		if !ok {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}

// MinInterval returns the minimum polling interval for ADS-B (5 seconds).
// readsb updates at 1Hz but polling at 5s matches display update latency.
func (f *Fetcher) MinInterval() time.Duration {
	return 5 * time.Second
}

func normalize(ac aircraft, now time.Time) (model.Aircraft, bool) {
	if ac.Hex == "" {
		return model.Aircraft{}, false
	}

	a := model.Aircraft{
		ICAO24:   strings.ToLower(ac.Hex),
		OnGround: ac.AltBaro.OnGround,
	}

	// Callsign
	cs := strings.TrimSpace(ac.Flight)
	if cs != "" {
		a.Callsign = &cs
	}

	// Position
	a.Lat = ac.Lat
	a.Lon = ac.Lon

	// Altitude (already in feet from readsb)
	if ac.AltBaro.Value != nil {
		a.AltFt = ac.AltBaro.Value
	} else if ac.AltGeom != nil {
		a.AltFt = ac.AltGeom
	}

	// Ground speed (already in knots from readsb)
	a.VelocityKt = ac.GS

	// Track/heading
	a.HeadingDeg = ac.Track

	// TimePosition: now minus "seen" seconds
	a.TimePosition = now.Add(-time.Duration(ac.Seen * float64(time.Second)))

	return a, true
}
