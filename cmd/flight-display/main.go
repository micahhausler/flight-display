package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/micahhausler/flight-display/internal/adsb"
	"github.com/micahhausler/flight-display/internal/aeroapi"
	"github.com/micahhausler/flight-display/internal/config"
	"github.com/micahhausler/flight-display/internal/fetch"
	"github.com/micahhausler/flight-display/internal/geo"
	"github.com/micahhausler/flight-display/internal/opensky"
	"github.com/micahhausler/flight-display/internal/render"
	"github.com/micahhausler/flight-display/internal/route"
	"github.com/micahhausler/flight-display/internal/tracker"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Load route database (optional — runs without it)
	var routeDB *route.DB
	if cfg.RoutesDir != "" {
		routeDB, err = route.LoadDB(cfg.RoutesDir)
		if err != nil {
			log.Printf("Warning: could not load route database: %v", err)
		}
	} else {
		log.Println("No routes_dir configured, running without route enrichment")
	}

	// Initialize fetcher based on configured source
	fetcher := newFetcher(cfg)

	// Initialize renderer and tracker
	renderer := render.NewStdout()
	trk := tracker.New(cfg.Observer, cfg.Aperture, cfg.SightingTTL, cfg.MaxRangeKM, routeDB)
	trk.SetFilters(cfg.MinAltFt, cfg.MinSpeedKt, cfg.CommercialOnly)

	// Set up AeroAPI route lookup if configured
	if cfg.AeroAPI.Key != "" {
		client := aeroapi.NewClient(cfg.AeroAPI.Key)
		cache := aeroapi.NewCache(client, cfg.AeroAPI.CachePath)
		trk.SetRouteLookup(cache)
		log.Printf("AeroAPI route lookup enabled (cache: %s)", cfg.AeroAPI.CachePath)
	}

	// Determine effective poll interval (config value floored by source minimum)
	pollInterval := cfg.PollInterval
	if min := fetcher.MinInterval(); pollInterval < min {
		pollInterval = min
	}

	log.Printf("Source: %s", cfg.Source)
	log.Printf("Observer: %.4f, %.4f (%.0fm MSL)", cfg.Observer.Lat, cfg.Observer.Lon, cfg.Observer.AltMSLMeter)
	log.Printf("Max range: %.0f km, Poll interval: %s, Sighting TTL: %s", cfg.MaxRangeKM, pollInterval, cfg.SightingTTL)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Poll loop
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Println("Starting flight display...")

	// Do an immediate first poll
	poll(fetcher, trk, renderer)

	for {
		select {
		case <-ticker.C:
			poll(fetcher, trk, renderer)
		case sig := <-sigCh:
			log.Printf("Received %v, shutting down", sig)
			return
		}
	}
}

func newFetcher(cfg *config.Config) fetch.Fetcher {
	switch cfg.Source {
	case "adsb":
		log.Printf("Using ADS-B source: %s", cfg.ADSB.URL)
		return adsb.NewFetcher(cfg.ADSB.URL)
	default:
		latMin, lonMin, latMax, lonMax := computeBBox(cfg)
		log.Printf("Using OpenSky source, bounding box: [%.2f, %.2f] to [%.2f, %.2f]", latMin, lonMin, latMax, lonMax)
		return opensky.NewFetcher(cfg.OpenSky.ClientID, cfg.OpenSky.ClientSecret, latMin, lonMin, latMax, lonMax)
	}
}

func poll(fetcher fetch.Fetcher, trk *tracker.Tracker, renderer render.Renderer) {
	aircraft, err := fetcher.Fetch()
	if err != nil {
		log.Printf("Fetch error: %v", err)
		return
	}
	if aircraft == nil {
		// Skipped poll (e.g., rate-limited) — don't update tracker
		return
	}

	events := trk.Process(aircraft)
	for _, event := range events {
		renderer.Render(event)
	}
}

// computeBBox computes a geographic bounding box large enough to contain all aircraft
// that could be visible through the aperture. Capped by max_range_km when configured.
func computeBBox(cfg *config.Config) (float64, float64, float64, float64) {
	var radiusM float64

	if cfg.MaxRangeKM > 0 {
		// Max range is the binding constraint — add a small margin for polling lag
		margin := 309.0 * cfg.PollInterval.Seconds() // ~600kt max aircraft speed
		radiusM = cfg.MaxRangeKM*1000 + margin
	} else {
		// No max range: derive from elevation angle + max altitude
		minElDeg := 90.0
		for _, r := range cfg.Aperture.Rects {
			if r.ElMin < minElDeg {
				minElDeg = r.ElMin
			}
		}
		if minElDeg < 0.5 {
			minElDeg = 0.5
		}

		maxAltM := 13716.0 // 45,000 ft
		altAboveObserver := maxAltM - cfg.Observer.AltMSLMeter
		groundDistM := altAboveObserver / math.Tan(minElDeg*math.Pi/180.0)
		margin := 309.0 * cfg.PollInterval.Seconds()
		radiusM = groundDistM + margin
	}

	return geo.BBoxDeg(cfg.Observer.Lat, cfg.Observer.Lon, radiusM)
}
