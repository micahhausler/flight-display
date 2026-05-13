package aeroapi

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
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
	Codeshares  []string    `json:"codeshares"`
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
// Returns the route and the resolved display callsign (marketing carrier ident).
// Returns nil, nil, nil if no route is found (not an error — just no data).
func (c *Client) LookupRoute(callsign string) (*model.Route, *string, error) {
	endpoint := fmt.Sprintf("%s/flights/%s", c.baseURL, url.PathEscape(callsign))

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("x-apikey", c.apiKey)

	q := req.URL.Query()
	q.Set("ident_type", "designator")
	q.Set("max_pages", "1")
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("aeroapi request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, nil // no such flight — not an error
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("aeroapi returned %d: %s", resp.StatusCode, string(body))
	}

	var data flightsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, nil, fmt.Errorf("decoding aeroapi response: %w", err)
	}

	// Find the most relevant flight (prefer one that is currently active / most recent)
	route, displayCS := extractRoute(data.Flights, callsign)
	return route, displayCS, nil
}

// extractRoute picks the best flight from the response and returns its route
// and display callsign. The display callsign is the ident from the AeroAPI response
// when it differs from the queried callsign (i.e., a codeshare resolution).
// Prefers flights that have departed but not arrived (in-flight), then most recent.
func extractRoute(flights []flight, queriedCallsign string) (*model.Route, *string) {
	if len(flights) == 0 {
		return nil, nil
	}

	// Find an in-flight entry (has actual_off, no actual_on)
	for _, f := range flights {
		if f.ActualOff != nil && f.ActualOn == nil {
			if r := routeFromFlight(f); r != nil {
				return r, displayCallsignFromFlight(f, queriedCallsign)
			}
		}
	}

	// Fall back to the first flight with origin/destination
	for _, f := range flights {
		if r := routeFromFlight(f); r != nil {
			return r, displayCallsignFromFlight(f, queriedCallsign)
		}
	}

	return nil, nil
}

// preferredCarriers is the priority order for selecting a marketing carrier
// from multiple codeshares that share the same flight number. Major US carriers
// are listed first since regional operators (SkyWest, Republic, Envoy, etc.)
// primarily fly under these brands.
var preferredCarriers = []string{"DAL", "ASA", "UAL", "AAL"}

// displayCallsignFromFlight resolves the marketing carrier callsign for a
// codeshare flight. Regional operators (e.g., SkyWest/SKW) broadcast their
// operating callsign on the transponder, but the meaningful identity for
// display is the marketing carrier (e.g., "ASA3113" for Alaska Air).
//
// Resolution logic:
//  1. Find codeshares sharing the same flight number as the queried callsign
//     (e.g., SKW3113 → ASA3113). This distinguishes brand-replacement codeshares
//     from interline partner codeshares (which use different flight numbers).
//  2. Among matches, prefer major US carriers (DAL, ASA, UAL, AAL) in priority order.
//  3. If no preferred carrier matches, use the first match.
//  4. Returns nil if no codeshares exist or none share the flight number
//     (i.e., this is a mainline flight, not a regional codeshare).
func displayCallsignFromFlight(f flight, queriedCallsign string) *string {
	if len(f.Codeshares) == 0 {
		return nil
	}

	queryNum := flightNumber(queriedCallsign)
	if queryNum == "" {
		return nil
	}

	// Collect codeshares with matching flight number but different airline prefix
	var matches []string
	queryPrefix := airlinePrefix(queriedCallsign)
	for _, cs := range f.Codeshares {
		csNum := flightNumber(cs)
		csPrefix := airlinePrefix(cs)
		if csNum == queryNum && !strings.EqualFold(csPrefix, queryPrefix) {
			matches = append(matches, cs)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	if len(matches) > 1 {
		log.Printf("Multiple codeshares match flight number %s for %s: %v", queryNum, queriedCallsign, matches)
	}

	// Prefer major US carriers in priority order
	for _, preferred := range preferredCarriers {
		for _, m := range matches {
			if strings.EqualFold(airlinePrefix(m), preferred) {
				return &m
			}
		}
	}

	// No preferred carrier found — use first match
	match := matches[0]
	return &match
}

// flightNumber extracts the numeric portion (with optional trailing alpha suffix)
// from an ICAO callsign. E.g., "SKW3113" → "3113", "DAL1921" → "1921".
func flightNumber(callsign string) string {
	cs := strings.TrimSpace(callsign)
	// Find where digits start (skip 2-3 letter airline prefix)
	for i, c := range cs {
		if c >= '0' && c <= '9' {
			return cs[i:]
		}
	}
	return ""
}

// airlinePrefix extracts the 2-3 letter ICAO airline code from a callsign.
// E.g., "SKW3113" → "SKW", "DAL1921" → "DAL".
func airlinePrefix(callsign string) string {
	cs := strings.TrimSpace(callsign)
	for i, c := range cs {
		if c >= '0' && c <= '9' {
			return cs[:i]
		}
	}
	return cs
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
