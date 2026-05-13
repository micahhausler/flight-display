package aeroapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

const defaultBaseURL = "https://aeroapi.flightaware.com/aeroapi"

// flightsResponse is the AeroAPI response for GET /flights/{ident}.
type flightsResponse struct {
	Flights []flight `json:"flights"`
}

type flight struct {
	Ident       string      `json:"ident"`
	IdentICAO   *string     `json:"ident_icao"`
	Origin      *airportRef `json:"origin"`
	Destination *airportRef `json:"destination"`
	ActualOff   *string     `json:"actual_off"`
	ActualOn    *string     `json:"actual_on"`
}

type airportRef struct {
	Code     *string `json:"code"`
	CodeICAO *string `json:"code_icao"`
	CodeIATA *string `json:"code_iata"`
}

// Client calls the FlightAware AeroAPI to look up flight routes.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates an AeroAPI client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NewClientWithBaseURL creates a client with a custom base URL (for testing).
func NewClientWithBaseURL(apiKey, baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// LookupRoute queries AeroAPI for the route of the given callsign.
// Returns nil, nil if no route is found (not an error — just no data).
func (c *Client) LookupRoute(callsign string) (*model.Route, error) {
	endpoint := fmt.Sprintf("%s/flights/%s", c.baseURL, url.PathEscape(callsign))

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-apikey", c.apiKey)

	q := req.URL.Query()
	q.Set("ident_type", "designator")
	q.Set("max_pages", "1")
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aeroapi request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no such flight — not an error
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aeroapi returned %d: %s", resp.StatusCode, string(body))
	}

	var data flightsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding aeroapi response: %w", err)
	}

	// Find the most relevant flight (prefer one that is currently active / most recent)
	route := extractRoute(data.Flights)
	return route, nil
}

// extractRoute picks the best flight from the response and returns its route.
// Prefers flights that have departed but not arrived (in-flight), then most recent.
func extractRoute(flights []flight) *model.Route {
	if len(flights) == 0 {
		return nil
	}

	// Find an in-flight entry (has actual_off, no actual_on)
	for _, f := range flights {
		if f.ActualOff != nil && f.ActualOn == nil {
			if r := routeFromFlight(f); r != nil {
				return r
			}
		}
	}

	// Fall back to the first flight with origin/destination
	for _, f := range flights {
		if r := routeFromFlight(f); r != nil {
			return r
		}
	}

	return nil
}

func routeFromFlight(f flight) *model.Route {
	origin := airportCode(f.Origin)
	dest := airportCode(f.Destination)
	if origin == "" || dest == "" {
		return nil
	}
	return &model.Route{Airports: []string{origin, dest}}
}

// airportCode returns the best display code for an airport (prefer IATA, fall back to ICAO, then generic code).
func airportCode(ref *airportRef) string {
	if ref == nil {
		return ""
	}
	if ref.CodeIATA != nil && *ref.CodeIATA != "" {
		return *ref.CodeIATA
	}
	if ref.CodeICAO != nil && *ref.CodeICAO != "" {
		return *ref.CodeICAO
	}
	if ref.Code != nil && *ref.Code != "" {
		return *ref.Code
	}
	return ""
}
