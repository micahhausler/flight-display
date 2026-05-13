package idle

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

const (
	weatherRefreshInterval = 15 * time.Minute
	weatherHTTPTimeout     = 2 * time.Second
	weatherStaleAfter      = 1 * time.Hour
)

// WeatherProvider fetches temperature from open-meteo.com in a background goroutine.
// Current() is always non-blocking.
type WeatherProvider struct {
	lat float64
	lon float64

	mu        sync.RWMutex
	cached    model.IdleInfo
	valid     bool
	fetchedAt time.Time
}

func NewWeatherProvider(lat, lon float64) *WeatherProvider {
	return &WeatherProvider{lat: lat, lon: lon}
}

func (p *WeatherProvider) Name() string { return "weather" }

// Start begins background weather refresh. Call with a cancellable context.
func (p *WeatherProvider) Start(ctx context.Context) {
	go p.refreshLoop(ctx)
}

func (p *WeatherProvider) Current() (model.IdleInfo, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.valid {
		return model.IdleInfo{}, false
	}
	// Invalidate if we haven't refreshed successfully in over an hour
	if time.Since(p.fetchedAt) > weatherStaleAfter {
		return model.IdleInfo{}, false
	}
	return p.cached, true
}

func (p *WeatherProvider) refreshLoop(ctx context.Context) {
	// Initial fetch
	p.refresh(ctx)

	ticker := time.NewTicker(weatherRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refresh(ctx)
		}
	}
}

func (p *WeatherProvider) refresh(ctx context.Context) {
	tempC, err := p.fetchTemperature(ctx)
	if err != nil {
		// Keep stale cache rather than clearing — staleAfter handles expiration
		return
	}

	tempF := tempC*9.0/5.0 + 32.0

	info := model.IdleInfo{
		Icon:    model.IconTemperature,
		Primary: fmt.Sprintf("%d°F / %d°C", int(math.Round(tempF)), int(math.Round(tempC))),
	}

	p.mu.Lock()
	p.cached = info
	p.valid = true
	p.fetchedAt = time.Now()
	p.mu.Unlock()
}

// openMeteoResponse is the subset of the open-meteo API response we need.
type openMeteoResponse struct {
	CurrentWeather struct {
		Temperature float64 `json:"temperature"`
	} `json:"current_weather"`
}

func (p *WeatherProvider) fetchTemperature(ctx context.Context) (float64, error) {
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f&current_weather=true",
		p.lat, p.lon,
	)

	ctx, cancel := context.WithTimeout(ctx, weatherHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	var result openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding open-meteo response: %w", err)
	}

	return result.CurrentWeather.Temperature, nil
}
