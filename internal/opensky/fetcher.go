package opensky

import (
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

// Fetcher wraps the OpenSky Client to satisfy the fetch.Fetcher interface.
type Fetcher struct {
	client                         *Client
	latMin, lonMin, latMax, lonMax float64
}

// NewFetcher creates an OpenSky Fetcher with the given bounding box.
func NewFetcher(clientID, clientSecret string, latMin, lonMin, latMax, lonMax float64) *Fetcher {
	return &Fetcher{
		client: NewClient(clientID, clientSecret),
		latMin: latMin,
		lonMin: lonMin,
		latMax: latMax,
		lonMax: lonMax,
	}
}

// Fetch retrieves aircraft state vectors from OpenSky within the bounding box.
// Returns nil, nil on rate-limit (skipped poll).
func (f *Fetcher) Fetch() ([]model.Aircraft, error) {
	resp, err := f.client.FetchStates(f.latMin, f.lonMin, f.latMax, f.lonMax)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil // rate limited
	}
	return Normalize(resp), nil
}

// MinInterval returns the minimum polling interval for OpenSky (10 seconds).
func (f *Fetcher) MinInterval() time.Duration {
	return 10 * time.Second
}
