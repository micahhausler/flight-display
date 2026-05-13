package aeroapi

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

const cacheTTL = 31 * 24 * time.Hour

// cacheEntry stores a route lookup result (including negative results).
type cacheEntry struct {
	Route     *model.Route `json:"route"`      // nil means "looked up, no route found"
	FetchedAt time.Time    `json:"fetched_at"`
}

// Cache provides a disk-backed route lookup cache fronting the AeroAPI client.
// Entries expire after 31 days. Negative results (no route) are also cached.
type Cache struct {
	client  *Client
	path    string
	mu      sync.Mutex
	entries map[string]cacheEntry
}

// NewCache creates a new disk-backed cache at the given path, backed by the given client.
// It loads existing cache entries from disk on creation.
func NewCache(client *Client, path string) *Cache {
	c := &Cache{
		client:  client,
		path:    path,
		entries: make(map[string]cacheEntry),
	}
	c.load()
	return c
}

// Lookup returns the route for a callsign. Checks disk cache first, calls AeroAPI on miss.
// Returns nil, nil for a cached or fresh negative result (no route exists for this callsign).
// Returns nil, error only on API failures.
func (c *Cache) Lookup(callsign string) (*model.Route, error) {
	c.mu.Lock()
	entry, ok := c.entries[callsign]
	c.mu.Unlock()

	if ok && time.Since(entry.FetchedAt) < cacheTTL {
		return entry.Route, nil
	}

	// Cache miss or stale — call the API
	route, err := c.client.LookupRoute(callsign)
	if err != nil {
		return nil, err
	}

	// Store result (including nil for negative cache)
	c.mu.Lock()
	c.entries[callsign] = cacheEntry{
		Route:     route,
		FetchedAt: time.Now(),
	}
	c.mu.Unlock()

	c.save()
	return route, nil
}

// load reads the cache file from disk. If the file doesn't exist or is invalid, starts empty.
func (c *Cache) load() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return // file doesn't exist yet — start fresh
	}

	var entries map[string]cacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("Warning: could not parse route cache %s: %v", c.path, err)
		return
	}

	// Filter out expired entries on load
	now := time.Now()
	for k, v := range entries {
		if now.Sub(v.FetchedAt) < cacheTTL {
			c.entries[k] = v
		}
	}

	log.Printf("Loaded %d cached routes from %s", len(c.entries), c.path)
}

// save writes the cache to disk atomically.
func (c *Cache) save() {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.Unlock()

	if err != nil {
		log.Printf("Warning: could not marshal route cache: %v", err)
		return
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Warning: could not create cache directory %s: %v", dir, err)
		return
	}

	// Write to temp file then rename for atomicity
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("Warning: could not write route cache: %v", err)
		return
	}
	if err := os.Rename(tmp, c.path); err != nil {
		log.Printf("Warning: could not rename route cache: %v", err)
	}
}
