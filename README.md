# flight-display

An ambient display that shows aircraft visible from your window. You look outside, see a plane, and glance at the display to know what it is.

```
+ DAL196   ICN-SEA   2050ft
+ ASA375   SEA-SFO   1575ft
+ UAL2101  SFO-SEA   2875ft
```

## How it works

You define your observer position (lat/lon/altitude) and the angular range of your window (azimuth and elevation bounds). The system polls for aircraft state vectors, filters them by:

1. **Aperture** — is the aircraft's bearing and elevation from you within your window's view?
2. **Slant range** — is it close enough to actually see? (default 60 km)
3. **Altitude and speed** — is it actually flying, not taxiing or parked?

When an aircraft enters your view, it prints a line. When it leaves, it prints another. Route information (e.g., ICN-SEA) comes from the [FlightAware AeroAPI](https://www.flightaware.com/aeroapi/) with a 31-day disk cache. For codeshare flights (e.g., SkyWest operating as United), AeroAPI resolves the marketing carrier automatically — you see "UAL1234" instead of "SKW5123".

## Data sources

| Source | Description | Config |
|--------|-------------|--------|
| **ADS-B receiver** (recommended) | Local RTL-SDR dongle + [readsb](https://github.com/wiedehopf/readsb). Real-time, no rate limits. | `source: adsb` |
| **OpenSky Network** | Remote API, useful for testing without hardware. | `source: opensky` |

| Route source | Description | Config |
|--------------|-------------|--------|
| **AeroAPI** (recommended) | Live route data from FlightAware. $0.005/lookup, cached 31 days. PiAware feeders get $10/mo free. | `aeroapi.key: "..."` |
| **VRS standing data** | Static CSV database. Free, but often stale. Fallback when no API key set. | `routes_dir: path/to/csvs` |

## Install

```sh
go install github.com/micahhausler/flight-display/cmd/flight-display@latest
```

Or build from source:

```sh
git clone https://github.com/micahhausler/flight-display.git
cd flight-display
go build -o flight-display ./cmd/flight-display/
```

Cross-compile for Raspberry Pi:

```sh
GOOS=linux GOARCH=arm64 go build -o flight-display ./cmd/flight-display/
```

## Configuration

Copy the example config and edit it for your location:

```sh
cp config.example.yaml config.yaml
```

### Minimal config (ADS-B + AeroAPI)

```yaml
source: adsb

observer:
  lat: 47.6115
  lon: -122.3470
  alt_msl_meters: 55

aperture:
  rects:
    - az_min: 160
      az_max: 290
      el_min: -2
      el_max: 30

poll_interval: 5s
sighting_ttl: 60s
max_range_km: 60
min_alt_ft: 150
min_speed_kt: 50
commercial_only: true

adsb:
  url: "http://localhost:8080/?all"

aeroapi:
  key: "your-aeroapi-key"

quiet_hours:
  sun: { start: "00:00", end: "06:30" }
  mon: { start: "22:00", end: "06:00" }
  tue: { start: "22:00", end: "06:00" }
  wed: { start: "22:00", end: "06:00" }
  thu: { start: "22:00", end: "06:00" }
  fri: { start: "23:00", end: "07:00" }
  sat: { start: "23:00", end: "07:00" }
```

### OpenSky config (no hardware required)

```yaml
source: opensky

observer:
  lat: 47.6115
  lon: -122.3470
  alt_msl_meters: 55

aperture:
  rects:
    - az_min: 160
      az_max: 290
      el_min: -2
      el_max: 30

poll_interval: 30s
max_range_km: 60

opensky:
  client_id: ""
  client_secret: ""
```

### Finding your aperture

The aperture defines what your window can see as compass bearings (azimuth) and angles above the horizon (elevation).

- **Azimuth**: Use a compass app on your phone. Stand at the window and note the bearing to the left edge and right edge of your view. `az_min` is the left edge, `az_max` is the right edge, going clockwise.
- **Elevation**: 0° is the horizon, 90° is straight up. Most windows span roughly -2° to 30°.
- **Multiple rectangles**: If your view wraps around a corner or has obstructions, use multiple rects.

Start with generous ranges and narrow them based on which displayed flights you can actually see.

### Quiet hours

Suppress polling on a per-day schedule. During quiet hours the display shows a clock instead of flight data:

```
🌙 22:37
```

Configure with a map of day names to time windows:

```yaml
quiet_hours:
  sun: { start: "00:00", end: "06:30" }
  mon: { start: "22:00", end: "06:00" }
  tue: { start: "22:00", end: "06:00" }
  wed: { start: "22:00", end: "06:00" }
  thu: { start: "22:00", end: "06:00" }
  fri: { start: "23:00", end: "07:00" }
  sat: { start: "23:00", end: "07:00" }
```

A window belongs to its start day — `mon: {start: "22:00", end: "06:00"}` means Monday 22:00 through Tuesday 06:00. Days without an entry have no quiet hours. Omit `quiet_hours` entirely to disable.

## Raspberry Pi deployment

Setup scripts are included for deploying on a Raspberry Pi with an RTL-SDR dongle:

```sh
# 1. Install readsb (ADS-B decoder) — builds from source, creates systemd service
ssh pi 'sudo bash -s -- --lat 47.6115 --lon -122.3470' < setup-adsb.sh

# 2. (Optional) Install PiAware to feed FlightAware and get free AeroAPI credit
ssh pi 'sudo bash -s' < setup-piaware.sh

# 3. Deploy the binary
GOOS=linux GOARCH=arm64 go build -o flight-display ./cmd/flight-display/
scp flight-display pi:~/bin/flight-display
scp config.yaml pi:~/config.yaml

# 4. Run
ssh pi '~/bin/flight-display -config ~/config.yaml'
```

### Hardware

- Raspberry Pi 3B+ or newer (arm64)
- [Nooelec NESDR Mini](https://www.amazon.com/dp/B009U7WZCA) (or any RTL2832U-based USB dongle)
- Antenna (included with the dongle, or a dedicated 1090 MHz antenna for better range)

## Usage

```sh
./flight-display -config config.yaml
```

Output:

```
+ DAL196   ICN-SEA   2050ft    # Delta 196 entered your view
+ ASA375   SEA-SFO   1575ft    # Alaska 375 entered
- DAL196   ICN-SEA             # Delta 196 left your view
🌙 22:00                        # Quiet hours started
🌙 22:01
```

When no aircraft are in view, the display rotates through ambient info every 15 seconds:

```
  🕙 3:45 PM
  📅 Tue May 13
  🌅🔽 @ 8:45pm
  🌡️ 72°F / 22°C
```

This is configurable via the `idle` section in config. Disable with `idle.enabled: false`.

## License

MIT
