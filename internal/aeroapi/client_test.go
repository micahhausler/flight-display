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
	route, err := client.LookupRoute("UAL2101")
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
}

func TestLookupRouteNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, err := client.LookupRoute("N12345")
	if err != nil {
		t.Fatalf("expected nil error for 404, got: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route for 404, got %v", route)
	}
}

func TestLookupRouteEmptyFlights(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"flights": []}`))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	route, err := client.LookupRoute("GHOST1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route for empty flights, got %v", route)
	}
}

func TestLookupRouteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"title":"error","reason":"internal","detail":"oops","status":500}`))
	}))
	defer srv.Close()

	client := NewClientWithBaseURL("test-key", srv.URL)
	_, err := client.LookupRoute("UAL1")
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
	route, err := client.LookupRoute("ASA375")
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
	route, err := cache.Lookup("DAL100")
	if err != nil {
		t.Fatalf("first lookup error: %v", err)
	}
	if route == nil || route.Airports[1] != "ATL" {
		t.Fatalf("first lookup: expected SEA-ATL, got %v", route)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second lookup — should hit cache, no API call
	route, err = cache.Lookup("DAL100")
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if route == nil || route.Airports[1] != "ATL" {
		t.Fatalf("second lookup: expected SEA-ATL, got %v", route)
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
	route, err = cache2.Lookup("DAL100")
	if err != nil {
		t.Fatalf("cache2 lookup error: %v", err)
	}
	if route == nil || route.Airports[1] != "ATL" {
		t.Fatalf("cache2 lookup: expected SEA-ATL from disk, got %v", route)
	}
	if callCount != 1 {
		t.Fatalf("expected still 1 API call after disk cache load, got %d", callCount)
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
	route, err := cache.Lookup("N12345")
	if err != nil {
		t.Fatalf("first lookup error: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route, got %v", route)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call, got %d", callCount)
	}

	// Second lookup — should be cached negative, no API call
	route, err = cache.Lookup("N12345")
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if route != nil {
		t.Fatalf("expected nil route from cache, got %v", route)
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
	route, err := cache.Lookup("AAL1")
	if err != nil {
		t.Fatalf("lookup error: %v", err)
	}
	if route == nil {
		t.Fatal("expected route, got nil")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 API call for stale entry, got %d", callCount)
	}
}
