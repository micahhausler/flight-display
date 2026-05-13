package aeroapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

func TestLookupRoute(t *testing.T) {
	response := `{
		"flights": [
			{
				"ident": "UAL2101",
				"ident_icao": "UAL2101",
				"origin": {
					"code": "KSEA",
					"code_icao": "KSEA",
					"code_iata": "SEA",
					"name": "Seattle-Tacoma Intl",
					"city": "Seattle"
				},
				"destination": {
					"code": "KSFO",
					"code_icao": "KSFO",
					"code_iata": "SFO",
					"name": "San Francisco Intl",
					"city": "San Francisco"
				},
				"actual_off": "2026-05-13T14:00:00Z",
				"actual_on": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if r.Header.Get("x-apikey") != "test-key" {
			t.Errorf("expected x-apikey=test-key, got %q", r.Header.Get("x-apikey"))
		}
		// Verify ident_type param
		if r.URL.Query().Get("ident_type") != "designator" {
			t.Errorf("expected ident_type=designator, got %q", r.URL.Query().Get("ident_type"))
		}
		// Verify path
		if r.URL.Path != "/flights/UAL2101" {
			t.Errorf("expected path /flights/UAL2101, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, displayCS, err := client.LookupRoute("UAL2101")
	if err != nil {
		t.Fatalf("LookupRoute() error: %v", err)
	}
	if route == nil {
		t.Fatal("expected route, got nil")
	}
	if len(route.Airports) != 2 {
		t.Fatalf("expected 2 airports, got %d", len(route.Airports))
	}
	if route.Airports[0] != "SEA" {
		t.Errorf("origin = %q, want SEA", route.Airports[0])
	}
	if route.Airports[1] != "SFO" {
		t.Errorf("destination = %q, want SFO", route.Airports[1])
	}
	// ident matches queried callsign — no display callsign enrichment
	if displayCS != nil {
		t.Errorf("expected nil displayCallsign when ident matches query, got %q", *displayCS)
	}
}

func TestLookupRouteCodeshare(t *testing.T) {
	// SkyWest operating as United: query SKW5123, response has codeshares with UAL5123
	response := `{
		"flights": [
			{
				"ident": "SKW5123",
				"ident_icao": "SKW5123",
				"codeshares": ["UAL5123"],
				"origin": {"code_iata": "SEA"},
				"destination": {"code_iata": "SFO"},
				"actual_off": "2026-05-13T14:00:00Z",
				"actual_on": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, displayCS, err := client.LookupRoute("SKW5123")
	if err != nil {
		t.Fatalf("LookupRoute() error: %v", err)
	}
	if route == nil {
		t.Fatal("expected route, got nil")
	}
	if route.Airports[0] != "SEA" || route.Airports[1] != "SFO" {
		t.Errorf("route = %v, want [SEA SFO]", route.Airports)
	}
	if displayCS == nil {
		t.Fatal("expected displayCallsign for codeshare, got nil")
	}
	if *displayCS != "UAL5123" {
		t.Errorf("displayCallsign = %q, want UAL5123", *displayCS)
	}
}

func TestLookupRouteCodeshareMultiple(t *testing.T) {
	// SKW3113 with multiple codeshares — ASA3113 shares flight number, others don't.
	// Should select ASA3113 (preferred carrier with matching flight number).
	response := `{
		"flights": [
			{
				"ident": "SKW3113",
				"ident_icao": "SKW3113",
				"codeshares": ["THT2309", "QTR2012", "ICE7624", "CFG5026", "ASA3113"],
				"origin": {"code_iata": "LAX"},
				"destination": {"code_iata": "SEA"},
				"actual_off": "2026-05-13T17:32:00Z",
				"actual_on": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, displayCS, err := client.LookupRoute("SKW3113")
	if err != nil {
		t.Fatalf("LookupRoute() error: %v", err)
	}
	if route == nil {
		t.Fatal("expected route, got nil")
	}
	if displayCS == nil {
		t.Fatal("expected displayCallsign for codeshare, got nil")
	}
	if *displayCS != "ASA3113" {
		t.Errorf("displayCallsign = %q, want ASA3113 (preferred carrier with matching flight number)", *displayCS)
	}
}

func TestLookupRouteMainlineNoRewrite(t *testing.T) {
	// UAL1234 operated by United with Copa as a partner codeshare (different flight number).
	// Should NOT rewrite — this is a mainline flight, not a regional codeshare.
	response := `{
		"flights": [
			{
				"ident": "UAL1234",
				"ident_icao": "UAL1234",
				"codeshares": ["CMP1205"],
				"origin": {"code_iata": "SEA"},
				"destination": {"code_iata": "SFO"},
				"actual_off": "2026-05-13T14:00:00Z",
				"actual_on": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	_, displayCS, err := client.LookupRoute("UAL1234")
	if err != nil {
		t.Fatalf("LookupRoute() error: %v", err)
	}
	// Copa's flight number (1205) differs from United's (1234) — no rewrite
	if displayCS != nil {
		t.Errorf("expected nil displayCallsign for mainline flight, got %q", *displayCS)
	}
}

func TestLookupRouteNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, displayCS, err := client.LookupRoute("N12345")
	if err != nil {
		t.Fatalf("expected nil error for 404, got: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route for 404, got %v", route)
	}
	if displayCS != nil {
		t.Fatalf("expected nil displayCallsign for 404, got %v", *displayCS)
	}
}

func TestLookupRouteEmptyFlights(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"flights": []}`))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, displayCS, err := client.LookupRoute("GHOST1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route for empty flights, got %v", route)
	}
	if displayCS != nil {
		t.Fatalf("expected nil displayCallsign for empty flights, got %v", *displayCS)
	}
}

func TestLookupRouteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"title":"error","reason":"internal","detail":"oops","status":500}`))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	_, _, err := client.LookupRoute("UAL1")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}

func TestLookupPrefersInFlight(t *testing.T) {
	// Two flights: one completed, one in-flight. Should prefer in-flight.
	response := `{
		"flights": [
			{
				"ident": "ASA375",
				"origin": {"code_iata": "SEA"},
				"destination": {"code_iata": "PDX"},
				"actual_off": "2026-05-12T10:00:00Z",
				"actual_on": "2026-05-12T11:00:00Z"
			},
			{
				"ident": "ASA375",
				"origin": {"code_iata": "SEA"},
				"destination": {"code_iata": "SFO"},
				"actual_off": "2026-05-13T10:00:00Z",
				"actual_on": null
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, _, err := client.LookupRoute("ASA375")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route == nil {
		t.Fatal("expected route, got nil")
	}
	// Should pick SEA-SFO (the in-flight one), not SEA-PDX (the completed one)
	if route.Airports[1] != "SFO" {
		t.Errorf("expected SFO (in-flight), got %q", route.Airports[1])
	}
}

func TestCacheDiskPersistence(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"flights":[{"ident":"DAL100","origin":{"code_iata":"SEA"},"destination":{"code_iata":"ATL"},"actual_off":"2026-05-13T10:00:00Z","actual_on":null}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "routes.json")

	client := NewClientWithBaseURL("test-key", srv.URL)
	cache := NewCache(client, cachePath)

	// First lookup — should call API
	info, err := cache.Lookup("DAL100")
	if err != nil {
		t.Fatalf("first lookup error: %v", err)
	}
	if info == nil || info.Route == nil || info.Route.Airports[1] != "ATL" {
		t.Fatalf("first lookup: expected SEA-ATL, got %v", info)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second lookup — should hit cache, no API call
	info, err = cache.Lookup("DAL100")
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if info == nil || info.Route == nil || info.Route.Airports[1] != "ATL" {
		t.Fatalf("second lookup: expected SEA-ATL, got %v", info)
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 API call after cache hit, got %d", callCount)
	}

	// Verify file was written to disk
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("cache file not written: %v", err)
	}
	var entries map[string]cacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("cache file invalid json: %v", err)
	}
	if _, ok := entries["DAL100"]; !ok {
		t.Fatal("DAL100 not in cache file")
	}

	// Create a new cache instance from the same file — should load from disk
	cache2 := NewCache(client, cachePath)
	info, err = cache2.Lookup("DAL100")
	if err != nil {
		t.Fatalf("cache2 lookup error: %v", err)
	}
	if info == nil || info.Route == nil || info.Route.Airports[1] != "ATL" {
		t.Fatalf("cache2 lookup: expected SEA-ATL from disk, got %v", info)
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 API call after disk cache load, got %d", callCount)
	}
}

func TestCacheDiskPersistsDisplayCallsign(t *testing.T) {
	// Verify that the display callsign survives disk persistence
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"flights":[{"ident":"SKW5678","codeshares":["ASA5678"],"origin":{"code_iata":"SEA"},"destination":{"code_iata":"LAX"},"actual_off":"2026-05-13T10:00:00Z","actual_on":null}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "routes.json")

	client := NewClientWithBaseURL("test-key", srv.URL)
	cache := NewCache(client, cachePath)

	// Lookup a codeshare — SKW5678 resolves to ASA5678
	info, err := cache.Lookup("SKW5678")
	if err != nil {
		t.Fatalf("lookup error: %v", err)
	}
	if info == nil || info.Route == nil {
		t.Fatal("expected route")
	}
	if info.DisplayCallsign == nil || *info.DisplayCallsign != "ASA5678" {
		t.Fatalf("expected displayCallsign ASA5678, got %v", info.DisplayCallsign)
	}

	// Load from disk in a new cache instance
	cache2 := NewCache(client, cachePath)
	info2, err := cache2.Lookup("SKW5678")
	if err != nil {
		t.Fatalf("cache2 lookup error: %v", err)
	}
	if info2 == nil || info2.Route == nil || info2.Route.Airports[0] != "SEA" {
		t.Fatalf("expected SEA-LAX from disk cache, got %v", info2)
	}
	if info2.DisplayCallsign == nil || *info2.DisplayCallsign != "ASA5678" {
		t.Fatalf("expected displayCallsign ASA5678 from disk cache, got %v", info2.DisplayCallsign)
	}
}

func TestCacheNegativeResult(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "routes.json")

	client := NewClientWithBaseURL("test-key", srv.URL)
	cache := NewCache(client, cachePath)

	// First lookup — API returns 404
	info, err := cache.Lookup("N12345")
	if err != nil {
		t.Fatalf("first lookup error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil info, got %v", info)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second lookup — should be cached negative, no API call
	info, err = cache.Lookup("N12345")
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if info != nil {
		t.Fatalf("expected nil info from cache, got %v", info)
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 API call after negative cache hit, got %d", callCount)
	}
}

func TestCacheExpiry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"flights":[{"ident":"AAL1","origin":{"code_iata":"DFW"},"destination":{"code_iata":"LAX"},"actual_off":"2026-05-13T10:00:00Z","actual_on":null}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "routes.json")

	client := NewClientWithBaseURL("test-key", srv.URL)
	cache := NewCache(client, cachePath)

	// Manually insert a stale entry
	cache.mu.Lock()
	cache.entries["AAL1"] = cacheEntry{
		Route:     &model.Route{Airports: []string{"DFW", "LAX"}},
		FetchedAt: time.Now().Add(-32 * 24 * time.Hour), // 32 days ago — expired
	}
	cache.mu.Unlock()

	// Lookup should bypass stale cache and call API
	info, err := cache.Lookup("AAL1")
	if err != nil {
		t.Fatalf("lookup error: %v", err)
	}
	if info == nil || info.Route == nil {
		t.Fatal("expected route, got nil")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call for stale entry, got %d", callCount)
	}
}
