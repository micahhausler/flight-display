package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Observer struct {
	Lat         float64 `yaml:"lat"`
	Lon         float64 `yaml:"lon"`
	AltMSLMeter float64 `yaml:"alt_msl_meters"`
}

type AzElRect struct {
	AzMin float64 `yaml:"az_min"`
	AzMax float64 `yaml:"az_max"`
	ElMin float64 `yaml:"el_min"`
	ElMax float64 `yaml:"el_max"`
}

type Aperture struct {
	Rects []AzElRect `yaml:"rects"`
}

type OpenSky struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type Config struct {
	Observer       Observer      `yaml:"observer"`
	Aperture       Aperture      `yaml:"aperture"`
	PollInterval   time.Duration `yaml:"poll_interval"`
	SightingTTL    time.Duration `yaml:"sighting_ttl"`
	MaxRangeKM     float64       `yaml:"max_range_km"`
	MinAltFt       float64       `yaml:"min_alt_ft"`
	MinSpeedKt     float64       `yaml:"min_speed_kt"`
	CommercialOnly bool          `yaml:"commercial_only"`
	OpenSky        OpenSky       `yaml:"opensky"`
	RoutesDir      string        `yaml:"routes_dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Observer.Lat < -90 || c.Observer.Lat > 90 {
		return fmt.Errorf("observer.lat must be in [-90, 90], got %v", c.Observer.Lat)
	}
	if c.Observer.Lon < -180 || c.Observer.Lon > 180 {
		return fmt.Errorf("observer.lon must be in [-180, 180], got %v", c.Observer.Lon)
	}
	if len(c.Aperture.Rects) == 0 {
		return fmt.Errorf("aperture must have at least one rect")
	}
	for i, r := range c.Aperture.Rects {
		if r.ElMin > r.ElMax {
			return fmt.Errorf("aperture.rects[%d]: el_min (%v) > el_max (%v)", i, r.ElMin, r.ElMax)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.PollInterval == 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.PollInterval < 10*time.Second {
		c.PollInterval = 10 * time.Second
	}
	if c.SightingTTL == 0 {
		c.SightingTTL = 60 * time.Second
	}
	if c.MaxRangeKM == 0 {
		c.MaxRangeKM = 60
	}
}
