# Agent Context: flight-display

## What this is

An ambient flight tracker for a Raspberry Pi. Identifies aircraft visible from a
window using a local ADS-B receiver, displays callsign + route + altitude on stdout
(LED matrix renderer planned).

## Architecture

```
RTL-SDR dongle → readsb (systemd) → HTTP API → flight-display → stdout
                                                     ↕
                                              AeroAPI (route lookup)
                                                     ↕
                                              ~/.cache/flight-display/routes.json
```

Pipeline stages:
1. **Fetcher** — polls aircraft data source (ADS-B or OpenSky). Returns `[]model.Aircraft`.
2. **Tracker** — filters by aperture/range/altitude/speed, manages sighting lifecycle, emits events.
3. **Renderer** — consumes events, writes output.

Route lookup happens synchronously in the tracker on Enter events.

## Key interfaces

- `fetch.Fetcher` — `Fetch() ([]Aircraft, error)` + `MinInterval() time.Duration`
- `render.Renderer` — `Render(event model.Event)`
- `tracker.RouteLookup` — `Lookup(callsign string) (*model.FlightInfo, error)`

## Packages

| Package | Responsibility |
|---------|---------------|
| `cmd/flight-display` | CLI entry point, config loading, wiring, poll loop |
| `internal/adsb` | ADS-B fetcher (reads readsb HTTP API) |
| `internal/aeroapi` | FlightAware AeroAPI client + disk-backed route cache |
| `internal/config` | YAML config parsing, validation, defaults |
| `internal/fetch` | Fetcher interface definition |
| `internal/geo` | Bearing, elevation, distance, bounding box math |
| `internal/model` | Aircraft, Sighting, Event, Route types |
| `internal/opensky` | OpenSky Network fetcher + normalizer |
| `internal/render` | Renderer interface + stdout implementation |
| `internal/route` | VRS static route database (fallback) |
| `internal/tracker` | Visibility filtering, sighting state, event emission |

## Build and test

```sh
go build ./...
go test ./...
GOOS=linux GOARCH=arm64 go build -o flight-display ./cmd/flight-display/  # Pi cross-compile
```

## Deployment target

Raspberry Pi 3B, Debian Bullseye arm64. The binary runs alongside:
- `readsb.service` — ADS-B decoder, listens on HTTP :8080 (API) and TCP :30005 (Beast)
- `piaware.service` — feeds FlightAware from Beast port

## Design decisions to preserve

- **Event-based rendering**, not snapshot-based. Tracker emits Enter/Update/Leave; renderers never diff.
- **Failed polls do not empty the tracker.** "We don't know" ≠ "nothing is there."
- **Identity keyed on ICAO24**, not callsign. Callsign can change or appear late.
- **TimePosition from the source's clock**, not ingestion wall-clock. Staleness is measured against the source.
- **Fetcher owns its rate semantics.** MinInterval() is a property of the source, not the poll loop.
- **AeroAPI is primary route source** when configured. VRS is fallback on API error only.
- **Negative cache entries** (callsign → nil) prevent repeated API calls for GA tail numbers.
- **Units fixed at the model layer.** Altitude is always feet, velocity always knots. Conversion happens in the fetcher/normalizer, nowhere else.

## Things NOT to change

- Do not modify the Makefile.
- Do not add CGO dependencies (breaks clean cross-compilation).
- Do not make the poll loop aware of source-specific details (OpenSky bounding boxes, readsb file paths, etc.). The Fetcher interface is the boundary.
- Do not add runtime dependencies beyond the Go standard library + gopkg.in/yaml.v3.

## Config

See `config.example.yaml`. Key fields:
- `source`: `"adsb"` or `"opensky"`
- `observer`: lat/lon/alt
- `aperture.rects[]`: azimuth/elevation bounds
- `aeroapi.key`: enables AeroAPI route lookup with 31-day disk cache
- `commercial_only`: filters to airline callsigns only (rejects GA N-numbers)

## Planned but not implemented

- **LED matrix renderer** (MAX7219 8-in-1, SPI, periph.io). Needs async queue + scroll goroutine.
- Display goes dark when queue is empty.
- Drop oldest on backpressure.
