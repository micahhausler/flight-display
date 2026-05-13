# Flight Display

An ambient display that shows the identity of aircraft visible from a window.

## Problem

You look out your window and see aircraft. You want to know what they are — airline,
flight number, altitude — without pulling out a phone app. A small always-on display
near the window shows this continuously.

## Concepts

### Observer

A fixed geographic point with an altitude, representing where a person is looking from.

```go
type Observer struct {
    Lat       float64 // WGS-84 decimal degrees
    Lon       float64 // WGS-84 decimal degrees
    AltMSL    float64 // meters above mean sea level
}
```

`AltMSL` is meters above mean sea level, not above ground. For a building, this is
ground elevation + floor height. Getting this wrong by 30 meters changes elevation
angles to nearby aircraft noticeably. In Seattle, ground elevation varies significantly
by neighborhood; use a topographic source, not a guess.

### Aperture

The angular region of sky visible through a window. Defined as a set of
azimuth-elevation rectangles in the observer's local frame. A window's visible region
is not a single rectangle — the building itself, the floor above, and nearby structures
occlude portions. Multiple rectangles compose to describe the actual visible region.

Azimuth is compass bearing in degrees (0 = north, 90 = east, 180 = south, 270 = west).
Elevation is angle above the horizon in degrees (0 = horizon, 90 = zenith).

```go
type AzElRect struct {
    AzMinDeg  float64 // inclusive
    AzMaxDeg  float64 // inclusive, may wrap past 360
    ElMinDeg  float64 // inclusive, >= 0
    ElMaxDeg  float64 // inclusive, <= 90
}

type Aperture struct {
    Rects []AzElRect
}
```

An aircraft is visible if its bearing and elevation from the observer fall inside any
rectangle in the aperture.

Azimuth wrapping: an aperture spanning northwest (e.g., 350 to 10 degrees) has
AzMinDeg > AzMaxDeg. The containment check handles this: if min > max, the aircraft
is inside when azimuth >= min OR azimuth <= max.

### Aircraft

A normalized representation of one aircraft at a moment in time.

```go
type Aircraft struct {
    ICAO24       string   // unique transponder address, hex (e.g. "a1b2c3"). Primary identity key.
    Callsign     *string  // e.g. "DL2873". Nil when not transmitted.
    Lat          *float64 // WGS-84 decimal degrees. Nil when position unknown.
    Lon          *float64 // WGS-84 decimal degrees. Nil when position unknown.
    AltFt        *float64 // barometric altitude in feet. Nil when unknown.
    HeadingDeg   *float64 // true track, degrees clockwise from north. Nil when unknown.
    VelocityKt   *float64 // ground speed in knots. Nil when unknown.
    OnGround     bool
    TimePosition time.Time // when the source last reported this position (from the source's clock, not ingestion time)
}
```

Pointer fields distinguish "unknown" from zero. An aircraft at sea level has
`AltFt` pointing to `0.0`; an aircraft with no altitude report has `AltFt == nil`.

`TimePosition` comes from the data source's own timestamp (OpenSky's `time_position`
field), not the wall clock at ingestion. This matters for TTL: an aircraft whose
position was last reported 45 seconds ago by the source is 45 seconds stale, regardless
of when the poll retrieved it. Without this, a stale position that OpenSky keeps
returning will extend its apparent freshness indefinitely.

Units are fixed at the type level. Altitude is always feet (converted from meters at
ingestion). Velocity is always knots (converted from m/s at ingestion). No unit
ambiguity propagates past the fetcher.

### Visibility

The computation that determines whether an aircraft is in the aperture and close
enough to see.

Given an observer and an aircraft with known position and altitude:

1. Compute the great-circle bearing from observer to aircraft (azimuth).
2. Compute the elevation angle: `atan2(altitude_difference, ground_distance)`.
3. Compute the slant range: `sqrt(ground_distance² + altitude_difference²)`.
4. Check if (azimuth, elevation) falls inside any rectangle in the aperture.
5. Check if slant range is within `max_range_km`.

The slant range filter is the primary distance control. A commercial jet at cruise
altitude (35,000 ft) is visible to the naked eye out to roughly 50-60 km in clear
conditions. Aircraft beyond this distance are in the angular aperture but not actually
visible from the window. Without a range limit, the system would display flights
hundreds of kilometers away that happen to be in the right bearing/elevation wedge.

The range limit also dramatically reduces the bounding box sent to OpenSky, which
lowers API credit cost (a ~120km box costs 1 credit vs. 3-4 for the uncapped box).

Aircraft without position (`Lat == nil || Lon == nil`) are never visible.
Aircraft on the ground (`OnGround == true`) are never visible.

Two additional filters suppress aircraft that are technically airborne per their
transponder but not meaningfully visible:

- **`min_alt_ft`**: Aircraft below this barometric altitude are suppressed. The
  `OnGround` flag from ADS-B is unreliable — aircraft taxiing or holding at the gate
  sometimes report as airborne. A minimum altitude relative to the local airport's
  field elevation catches these. For a sea-level observer near SEA (field elevation
  433 ft), a value of 500 ft filters ground operations cleanly.
- **`min_speed_kt`**: Aircraft below this ground speed are suppressed. A taxiing
  aircraft moves at 15-20 knots; anything in flight is doing 60+ knots. A threshold
  of 50 knots catches the remaining ground traffic that passes the altitude filter.

### Sighting

A flight that has entered the aperture and may still be in it.

```go
type Sighting struct {
    Aircraft     Aircraft
    BearingDeg   float64   // azimuth from observer, degrees
    ElevationDeg float64   // elevation from observer, degrees
    FirstSeen    time.Time // when this aircraft first entered the aperture
    LastPosition time.Time // most recent source-reported time_position while visible
}
```

A sighting is **active** when `time.Since(LastPosition) < TTL`. The TTL is 60 seconds.
`LastPosition` is the source's `time_position`, not wall-clock ingestion time. This
means the TTL tracks how stale the aircraft's actual position is, not how recently we
polled. An aircraft whose position hasn't been updated by the source for 60 seconds is
removed even if OpenSky keeps returning the stale record.

This smooths over gaps in API responses without meaningfully lying about what is in the
sky. When a sighting's TTL expires, it is removed and a "leave" event is emitted.

Identity is keyed on ICAO24, not callsign. Callsign can change or appear late;
ICAO24 is stable for the duration of a flight.

### Events

The system emits events when the set of visible aircraft changes.

```go
type EventKind int

const (
    Enter EventKind = iota // aircraft entered the aperture
    Update                 // aircraft still in aperture, state changed
    Leave                  // aircraft left the aperture or TTL expired
)

type Event struct {
    Kind     EventKind
    Sighting Sighting
}
```

The renderer receives events, not snapshots. This means:
- A STDOUT renderer prints a line on Enter, optionally on Update, and on Leave.
- A future LED matrix renderer can flash on entry, hold, and clear on exit.
- No renderer needs to diff the current set against the previous set.

### Route Enrichment

Route information (origin/destination airports) comes from the FlightAware AeroAPI.
When a new aircraft enters the aperture, its callsign is looked up via
`GET /flights/{ident}` with `ident_type=designator`. The response provides origin
and destination airports with IATA codes.

```go
type Route struct {
    Airports []string // IATA codes in order flown, e.g. ["SEA", "SFO"]
}
```

The AeroAPI response also provides the resolved flight identity (`ident` field).
For codeshare flights operated by regional carriers — e.g., SkyWest (SKW) operating
as United (UAL), Alaska (ASA), or Delta (DAL) — the response's `ident` is the
marketing carrier designator, not the operating carrier. The system captures this as
a display callsign when it differs from the queried (transponder) callsign.

```go
type FlightInfo struct {
    Route           *Route  // origin/destination airports; nil if unknown
    DisplayCallsign *string // marketing carrier ident (e.g. "ASA1234" for a SKW codeshare); nil = use raw callsign
}
```

The display callsign comparison uses case-insensitive matching with trimmed whitespace
to handle any normalization differences between AeroAPI and transponder callsigns.
When the response `ident` matches the queried callsign (non-codeshare flights),
`DisplayCallsign` is nil and the raw transponder callsign is used for display.

The AeroAPI lookup is backed by a disk-persisted cache with a 31-day TTL. The cache
stores both positive results (callsign → route + display callsign) and negative
results (callsign → nil, meaning "we checked, no data exists"). This prevents
repeated API calls for GA tail numbers that will never have a route.

Cache behavior:
- On Enter, check cache for callsign.
- Cache hit (< 31 days old): use cached result. No API call.
- Cache miss or stale: call AeroAPI, store result to disk, use it.
- API error: fall back to VRS static database if configured (no display callsign from VRS).

The cache is a single JSON file at `~/.cache/flight-display/routes.json`. It is
loaded at startup and written on each new entry. The dataset is small (a few hundred
entries after steady-state operation) and writes are infrequent (a few per day once
the cache is warm).

Cost: `GET /flights/{ident}` costs $0.005 per result set. With a 31-day cache and
a typical window seeing ~100 unique commercial callsigns per month, steady-state cost
is under $1/month. A PiAware ADS-B feeder gets $10/month in free API credit.

A fallback route source — the Virtual Radar Server (VRS) standing data — can be
configured via `routes_dir`. When an AeroAPI key is configured, it takes precedence;
VRS is only consulted on API failure. When no AeroAPI key is configured, VRS is used
directly as before. VRS provides routes only — no display callsign resolution.

The Sighting struct carries both the optional route and display callsign:

```go
type Sighting struct {
    Aircraft        Aircraft
    Route           *Route    // nil if no route found in any source
    DisplayCallsign *string   // marketing carrier ident; nil = use Aircraft.Callsign
    BearingDeg      float64
    ElevationDeg    float64
    FirstSeen       time.Time
    LastPosition    time.Time
}
```

### Display Format

The STDOUT renderer prints one line per Enter event:

```
+ DAL1921  SEA-BNA  12400ft
+ ASA1042  SEA-SFO   8200ft
+ N172SP             5400ft
```

For codeshare flights (regional operators like SkyWest flying as a major carrier),
the display shows the marketing carrier callsign when AeroAPI resolves it:

```
+ UAL1234  SEA-SFO   8200ft   (transponder: SKW5123, displayed as UAL1234)
```

Fields:
- **`+`** prefix indicates entry. `-` prefix indicates leave.
- **Callsign** (left-aligned, padded to 8 chars). Uses `DisplayCallsign` when
  available (codeshare resolution from AeroAPI), otherwise `Aircraft.Callsign`.
  If both are nil, the aircraft is suppressed.
- **Route** (IATA airport codes, hyphen-separated). Blank if not in route database.
- **Altitude** in feet, no decimal.

On Leave, the renderer prints:

```
- DAL1921  SEA-BNA
```

## Data Flow

```
┌─────────┐                        ┌──────────────┐     ┌──────────┐
│ Fetcher │───── []Aircraft ──────>│   Tracker    │────>│ Renderer │
└─────────┘                        └──────────────┘     └──────────┘
     ^                                    │
     │                                    │
  ADS-B receiver (readsb)       Visibility filter +
  or OpenSky API                sighting state +
                                event emission +
                                route lookup (AeroAPI)
```

The Fetcher interface abstracts the aircraft data source:

```go
type Fetcher interface {
    Fetch() ([]model.Aircraft, error)
    MinInterval() time.Duration
}
```

`Fetch()` returns the current set of aircraft. Returns nil, nil to signal a skipped
poll (e.g., rate-limited). `MinInterval()` returns the minimum polling interval the
source supports — the poll loop will not poll faster than this regardless of
configuration.

Two implementations exist:

- **ADS-B fetcher**: Queries a local readsb instance's HTTP API
  (`http://localhost:8080/?all`). The RTL-SDR dongle and readsb handle radio reception
  and Mode S decoding. The fetcher reads pre-aggregated aircraft state at 1Hz.
  MinInterval: 5 seconds.
- **OpenSky fetcher**: Queries the OpenSky Network REST API with a geographic bounding
  box. Handles OAuth2 token refresh and rate limiting. MinInterval: 10 seconds.

Each fetcher owns its own filtering semantics. The OpenSky fetcher applies a bounding
box server-side (query optimization). The ADS-B fetcher returns everything the antenna
receives — the antenna's physical range is the de facto filter.

Route lookup happens in the tracker at Enter time — when a new aircraft becomes
visible, its callsign is looked up via the route source (AeroAPI with disk cache,
falling back to VRS). The route is stored on the sighting and included in the Enter
event.

### Fetcher

The Fetcher interface decouples the data source from the tracker. Configuration
selects which implementation is active.

#### ADS-B Fetcher

Reads from a local readsb instance via its HTTP API (`GET /?all` on port 8080).
readsb handles RTL-SDR device interaction and ADS-B/Mode S decoding as a separate
systemd service. The fetcher normalizes readsb's JSON into `[]Aircraft`:

- `hex` → ICAO24
- `flight` → Callsign (trimmed)
- `lat`, `lon` → Lat, Lon
- `alt_baro` → AltFt (already in feet; can be the string "ground" indicating on-ground)
- `alt_geom` → fallback altitude (already in feet)
- `gs` → VelocityKt (already in knots)
- `track` → HeadingDeg
- `seen` → TimePosition (computed as `now - seen` seconds)

No unit conversion is needed — readsb outputs feet and knots natively.

The `alt_baro` field requires special handling: it is normally a number but can be
the string `"ground"` when the aircraft reports on-ground status. A custom JSON
unmarshaler handles this polymorphism.

MinInterval: 5 seconds. readsb updates at 1Hz but polling faster than 5s adds no
display value.

#### OpenSky Fetcher

Calls the OpenSky `/states/all` endpoint with a geographic bounding box. The bounding
box is a query optimization — it reduces the response size and API credit cost. It is
not a correctness boundary. The box must be large enough to contain all aircraft that
could be visible at the maximum altitude and elevation angle the aperture allows.

Bounding box sizing: when `max_range_km` is configured (the common case), the
bounding box radius is simply `max_range_km` plus a margin for aircraft movement
between polls. This produces a tight box (~120 km across for a 60 km range) that
costs 1 API credit per query.

The fetcher owns rate limiting. OpenSky anonymous access has 400 credits/day. An
active feeder gets 8,000 credits/day. Default poll interval: 30 seconds.

On HTTP 429 (rate limited), the fetcher returns nil, nil — signaling a skipped poll.
On other errors, it returns an error. The fetcher never crashes the process on
transient API failures.

Authentication: OAuth2 client credentials flow. Client ID and secret are provided via
config. If not provided, the fetcher operates anonymously (tighter limits).

MinInterval: 10 seconds.

### Normalizer

Each fetcher normalizes its source-specific response into `[]Aircraft`. This is where
unit conversions happen (if any):

- **OpenSky**: Altitude meters → feet (`m * 3.28084`), velocity m/s → knots
  (`ms * 1.94384`), callsign trimmed of whitespace (OpenSky pads to 8 chars).
  Barometric altitude (field index 7) with geometric altitude (index 13) as fallback.
- **ADS-B (readsb)**: No unit conversion needed. readsb outputs altitude in feet and
  ground speed in knots natively. Callsign trimmed of whitespace.

Aircraft with no ICAO24 are dropped by both normalizers.

### Tracker

Maintains the set of active sightings and emits events. Each poll cycle:

1. Receive `[]Aircraft` from the normalizer.
2. For each aircraft, compute visibility against the observer + aperture.
3. For visible aircraft:
   - If ICAO24 is not in the active set: create a sighting, emit `Enter`.
   - If ICAO24 is in the active set: update the sighting's `LastPosition`. Emit
     `Update` only if state changed materially — altitude changed by >200ft or bearing
     changed by >5 degrees. This is the tracker's responsibility; renderers receive
     pre-filtered events and do not need their own change detection.
4. For each active sighting not refreshed by this poll:
   - If `time.Since(LastPosition) >= TTL`: remove it, emit `Leave`.

The tracker does not call the fetcher. It receives aircraft data and produces events.
The poll loop connects them.

### Renderer

Consumes events and writes to an output. The STDOUT renderer is the V1 implementation.

The renderer interface is:

```go
type Renderer interface {
    Render(event Event)
}
```

The STDOUT renderer suppresses:
- Aircraft with nil callsign (no useful identifier to display).
- `Update` events (V1 only shows enter and leave).

### Poll Loop

The top-level orchestrator. Pseudocode:

```
interval = max(config.poll_interval, fetcher.MinInterval())
wasQuiet = false
lastClockMinute = -1

every <interval>:
    now = time.Now()
    if inQuietHours(now, config.quiet_hours):
        if !wasQuiet:
            // Transition into quiet hours: emit Leave for all, clear tracker
            leaveEvents = tracker.Clear()
            for event in leaveEvents:
                renderer.Render(event)
            wasQuiet = true
            lastClockMinute = -1
        if now.Minute() != lastClockMinute:
            print("🌙 " + now.Format("15:04"))
            lastClockMinute = now.Minute()
        continue
    if wasQuiet:
        wasQuiet = false
    aircraft, err = fetcher.Fetch()
    if err:
        log error
        // Do NOT pass empty list to tracker. A failed poll means
        // "we don't know" — not "the sky is empty."
        continue
    if aircraft == nil:
        // Skipped poll (e.g., rate-limited). Don't update tracker.
        continue
    events = tracker.Process(aircraft)
    for event in events:
        renderer.Render(event)
```

The poll loop does not know about OpenSky, ADS-B, visibility math, or display
formatting. Quiet hours is a time-based policy that belongs to the orchestrator —
it suppresses polling entirely rather than filtering results.

### Failure Semantics

The distinction between "we don't know" and "we know there's nothing" is load-bearing
for an ambient display.

- **Network error, HTTP 5xx, HTTP 429**: The poll failed. Do not update the tracker.
  Active sightings remain and age against their TTL. The display stays stable rather
  than flickering empty on every API hiccup.
- **Malformed JSON**: Treated as a failed poll. Log and continue.
- **Valid response with empty state list**: This is real data saying "no aircraft in the
  bounding box." Pass the empty list to the tracker. Active sightings whose
  `LastPosition` TTL expires will emit Leave events normally. This is the correct
  behavior — the source is telling us there is nothing there.
- **Valid response with aircraft but none visible**: Same as above — the tracker receives
  the aircraft, none pass the visibility filter, and sightings age out naturally.

## Configuration

A YAML file. Example for a Raspberry Pi with an ADS-B receiver near Seattle:

```yaml
source: adsb  # "adsb" or "opensky"

observer:
  lat: 47.6115
  lon: -122.3470
  alt_msl_meters: 55  # ground elevation + building floor height

aperture:
  rects:
    # Main window view: south through west
    - az_min: 160
      az_max: 290
      el_min: -2
      el_max: 30
    # Narrower view toward northwest (partial, past building edge)
    - az_min: 290
      az_max: 330
      el_min: 0
      el_max: 15

poll_interval: 5s
sighting_ttl: 60s
max_range_km: 60
min_alt_ft: 150
min_speed_kt: 50
commercial_only: true

adsb:
  url: "http://localhost:8080/?all"

aeroapi:
  key: ""  # FlightAware AeroAPI key; omit to use VRS database
  # cache_path: ~/.cache/flight-display/routes.json  # default

quiet_hours:
  # sun: { start: "00:00", end: "06:30" }
  # mon: { start: "22:00", end: "06:00" }

# opensky:
#   client_id: ""
#   client_secret: ""
```

The `source` field selects the aircraft data source. Default poll interval depends
on the source: 5 seconds for ADS-B, 30 seconds for OpenSky. The poll loop enforces
each source's MinInterval as a floor.

When `aeroapi.key` is set, route lookups use the FlightAware AeroAPI with a 31-day
disk cache. When empty, routes come from the VRS static database (configured via
`routes_dir`).

Negative elevation minimum allows for aircraft below the observer's altitude (e.g.,
low approaches to SEA visible from an elevated vantage point).

### Quiet Hours

Optional per-day-of-week schedule that suppresses polling and displays a clock instead.
Useful for nighttime when the display's output is unwanted.

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

Each key is a three-letter day abbreviation (`sun`/`mon`/`tue`/`wed`/`thu`/`fri`/`sat`).
Values are `{start, end}` in `HH:MM` 24-hour format. A window belongs to its start day:
`mon: {start: "22:00", end: "06:00"}` means Monday 22:00 through Tuesday 06:00.
Days without an entry have no quiet hours. Omitting `quiet_hours` entirely disables
the feature.

Behavior on entry:
1. All active sightings emit Leave events (tracker is cleared).
2. Display prints `🌙 HH:MM` once per minute.
3. No fetcher calls are made — zero API cost during quiet hours.

On exit, polling resumes immediately with a fresh tracker state.

### Aperture Calibration

Aperture ranges are initial estimates requiring empirical calibration. The deployment
process is:

1. Start with generous azimuth/elevation ranges.
2. Run the system and note which displayed flights are actually visible out the window
   vs. which are not.
3. Narrow the ranges to exclude false positives (flights reported but not visible).
4. Watch for false negatives (visible flights not reported) and expand if needed.

This is a real part of deployment, not a footnote. The numbers in the example config
are not surveyed — they are a starting point.

## Project Structure

```
flight-display/
  cmd/
    flight-display/
      main.go          # CLI entry point, config loading, poll loop
  internal/
    adsb/
      fetcher.go       # ADS-B fetcher: reads readsb HTTP API
    aeroapi/
      client.go        # FlightAware AeroAPI HTTP client
      cache.go         # Disk-backed 31-day route cache
    config/
      config.go        # YAML config parsing
      quiet.go         # Quiet hours predicate and time-window logic
    fetch/
      fetcher.go       # Fetcher interface definition
    geo/
      geo.go           # bearing, elevation, distance calculations
    opensky/
      client.go        # OpenSky HTTP client, auth, rate limiting
      fetcher.go       # OpenSky Fetcher implementation
      types.go         # OpenSky API response types
      normalize.go     # OpenSky response -> []Aircraft
    model/
      aircraft.go      # Aircraft, Sighting, Event, Route types
    route/
      db.go            # VRS route database (fallback)
      icao_to_iata.go  # ICAO airport code to IATA conversion
    tracker/
      tracker.go       # Visibility filtering, sighting state, event emission
    render/
      stdout.go        # STDOUT renderer
    idle/
      idle.go          # Provider interface, Rotator
      clock.go         # Clock and Date providers
      sun.go           # Sunrise/sunset (NOAA solar position)
      weather.go       # Temperature (open-meteo.com, background refresh)
  go.mod
  go.sum
```

## Idle Display

When no aircraft are visible (tracker active set is empty), the display rotates
through ambient information items instead of showing a blank screen. This is an
ambient display — it should always show something.

### Idle Providers

Each provider supplies one kind of information:

```go
type Provider interface {
    Name() string
    Current() (model.IdleInfo, bool) // false = no data, skip me
}
```

Four providers:
- **Clock** — current local time (e.g., "3:45 PM")
- **Date** — current date (e.g., "Tue May 13")
- **Sunrise/Sunset** — next sunrise or sunset computed from observer lat/lon using
  the NOAA simplified solar position algorithm. Pure math, no external calls.
  Format: "🌅🔼 @ 5:11am" (next sunrise) or "🌅🔽 @ 9:11pm" (next sunset)
- **Weather** — current temperature from open-meteo.com (free, no API key).
  Background goroutine refreshes every 15 minutes with a 2-second HTTP timeout.
  Cache expires after 1 hour of consecutive failures. `Current()` is non-blocking.

### Idle Event

Idle information flows through the existing `Render(event)` interface as a new
event kind:

```go
const Idle EventKind = 3

type IdleInfo struct {
    Icon    IdleIcon // IconClock, IconDate, IconSunrise, IconSunset, IconTemperature
    Primary string   // pre-formatted display value
}
```

The `Event` struct carries both `Sighting` and `IdleInfo` as a tagged union; `Kind`
determines which is meaningful.

### Rotation

A `Rotator` cycles through providers in round-robin. The poll loop drives rotation:
every N poll ticks (default: 3 ticks × 5s = 15 seconds), the rotator advances.
Unavailable providers are skipped. When the active set transitions from non-empty to
empty, the rotator resets and immediately renders the first item. When an aircraft
enters, idle rendering stops immediately.

### Configuration

```yaml
idle:
  enabled: true           # default: true
  rotate_seconds: 15      # rotation cadence
  providers:              # which providers to include (default: all)
    - clock
    - date
    - sunrise_sunset
    - weather
```

Providers that cannot operate (e.g., weather with no network) self-disable by
returning `false` from `Current()`. The rotator skips them.

## Scoped Out

- **LED matrix renderer**. The `Renderer` interface exists; a matrix implementation is
  additive. The target hardware is a MAX7219 8-in-1 dot matrix (8x64 pixels, SPI) on
  a Raspberry Pi. The renderer will need an internal event queue and a background
  goroutine driving scroll, since `Render()` must not block the poll loop.
- **"Will soon cross" prediction**. Trajectory prediction from heading and speed gives
  ~30-60 seconds of useful lookahead before altitude changes and ATC vectoring make it
  wrong. STAR/SID-aware prediction is a separate project. Deferred.
- **Web UI**.
- **Multi-user / multi-window**. The config supports one observer and one aperture. Running
  multiple instances with different configs is the multi-window story for now.

## Rejected Alternatives

**Geographic polygon for viewport.** A polygon on the ground does not capture what a
window actually sees. An aircraft at 35,000 feet that is visually in the window could be
100+ miles away geographically. The angular aperture model (azimuth + elevation from
observer) handles altitude correctly and matches the physical reality of a window — it is
a cone of directions, not a shape on a map.

**Callsign as identity key.** Callsign can be empty, change mid-flight, or appear
late. ICAO24 is the stable transponder address for the duration of a flight and is always
present in the OpenSky response.

**Snapshot-based rendering (current set per poll).** Forces every renderer to diff the
previous set against the current set to determine what changed. Event-based rendering
(enter/update/leave) pushes this responsibility into the tracker once, and renderers
receive pre-computed transitions.

**Pure-Go RTL-SDR / ADS-B decoding.** Reimplementing the DSP demodulation, Mode S frame
decoding, CRC, CPR position decoding, and aircraft state aggregation in Go would mean
owning a decoder stack. readsb already does this correctly and is battle-tested. Working
in the seam — letting readsb own device interaction and decoding, reading its HTTP API —
avoids this entire layer.

**Reading readsb's aircraft.json file from disk.** readsb writes this file to `/run`
(tmpfs), so SD card wear is not the concern. The HTTP API (`--net-api-port`) is the
supported interface — it's stable across installs, doesn't depend on filesystem layout
or the `--write-json` flag being set, and fails loudly if readsb is down rather than
silently reading a stale file.

**VRS static database as primary route source.** The VRS standing-data CSV files are
community-maintained and often stale or incomplete. AeroAPI provides live, accurate
route data for the current flight. With a 31-day disk cache, AeroAPI costs under
$1/month and provides better coverage — including regional operators and charter
flights that VRS misses entirely.

**In-memory route cache (no disk persistence).** A cold cache on every restart means
~100+ API calls each time the process starts. Disk persistence ensures the cache
survives restarts and accumulates over time, reducing API cost to near-zero in
steady state.

**Python.** Mature geo libraries, but the geo math here is basic trig — no need for
shapely or pyproj. Go provides a single binary, clean cross-compilation for Raspberry Pi,
and a natural polling model with goroutines.

**Bearing-from-observer in the display.** The bearing (azimuth from observer) tells
you roughly where in the window to look. Dropped because the window view is narrow
enough that "it's somewhere out there" is sufficient, and the bearing is more useful
as an internal visibility computation than as a display field. Route information is
more valuable display real estate.
