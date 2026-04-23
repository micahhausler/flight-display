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

Route information (origin/destination airports) is not available from the OpenSky
state vectors endpoint. It is sourced from the Virtual Radar Server (VRS) standing
data — a community-maintained, CC0-licensed database of callsign-to-route mappings.

The VRS route data is a set of CSV files keyed by ICAO callsign code (e.g., `DAL` for
Delta, `ASA` for Alaska). Each row maps a callsign to a hyphen-separated list of ICAO
airport codes:

```
DAL1921,DAL,1921,DAL,KSEA-KBNA
ASA1042,ASA,1042,ASA,KSEA-KSFO
```

The route database is loaded from local CSV files at startup into an in-memory map
keyed by normalized callsign. Lookup is per-sighting: when a new aircraft enters the
aperture, its callsign is normalized (using VRS's normalization rules — split into
code + number, strip leading zeros from number, swap IATA code for ICAO if needed)
and looked up in the map. The result is stored on the Sighting.

Coverage: the VRS database covers most scheduled commercial traffic. GA, military,
and charter flights will not have entries. This is acceptable — commercial traffic
dominates what is visible from this location, and missing routes degrade gracefully
(callsign + altitude is still displayed).

The ICAO airport codes from the database (e.g., `KSEA`) are converted to IATA codes
(e.g., `SEA`) for display. A small static mapping of the ~500 most common airports
covers this; unknown ICAO codes pass through as-is.

```go
type Route struct {
    Airports []string // IATA codes in order flown, e.g. ["SEA", "BNA"]
}
```

The Sighting struct gains an optional route:

```go
type Sighting struct {
    Aircraft     Aircraft
    Route        *Route    // nil if callsign not in route database
    BearingDeg   float64
    ElevationDeg float64
    FirstSeen    time.Time
    LastPosition time.Time
}
```

### Display Format

The STDOUT renderer prints one line per Enter event:

```
+ DAL1921  SEA-BNA  12400ft
+ ASA1042  SEA-SFO   8200ft
+ N172SP             5400ft
```

Fields:
- **`+`** prefix indicates entry. `-` prefix indicates leave.
- **Callsign** (left-aligned, padded to 8 chars). If callsign is nil, the aircraft is
  suppressed — a row with no identifier is noise for the ambient display use case.
- **Route** (IATA airport codes, hyphen-separated). Blank if not in route database.
- **Altitude** in feet, no decimal.

On Leave, the renderer prints:

```
- DAL1921  SEA-BNA
```

## Data Flow

```
┌─────────┐     ┌────────────┐     ┌──────────────┐     ┌──────────┐
│ Fetcher │────>│ Normalizer │────>│   Tracker    │────>│ Renderer │
└─────────┘     └────────────┘     └──────────────┘     └──────────┘
     ^                                    │
     │                                    │
  OpenSky API                    Visibility filter +
  (bounding box query)           sighting state +
                                 event emission +
                                 route lookup
```

Route lookup happens in the tracker at Enter time — when a new aircraft becomes
visible, its callsign is looked up in the route database. The route is stored on
the sighting and included in the Enter event. The route database is read-only after
startup; the tracker holds a reference to it but does not modify it.

### Fetcher

Calls the OpenSky `/states/all` endpoint with a geographic bounding box. The bounding
box is a query optimization — it reduces the response size and API credit cost. It is
not a correctness boundary. The box must be large enough to contain all aircraft that
could be visible at the maximum altitude and elevation angle the aperture allows.

Bounding box sizing: when `max_range_km` is configured (the common case), the
bounding box radius is simply `max_range_km` plus a margin for aircraft movement
between polls. This produces a tight box (~120 km across for a 60 km range) that
costs 1 API credit per query.

When `max_range_km` is not configured, the radius is derived from the aperture:

```
ground_distance = max_considered_altitude / tan(min_elevation_angle)
bbox_radius = ground_distance + (poll_interval * max_aircraft_speed)
```

This fallback produces a much larger box. The aperture and range filters handle
precision in either case.

The fetcher owns rate limiting. OpenSky anonymous access resolves state every 10 seconds
and has 400 credits/day for the states endpoint. A bounding box <= 25 sq degrees costs
1 credit. At one poll per 10 seconds, that's 8,640 credits/day — exceeding the anonymous
quota. Options:

- **Authenticated access** (standard user): 4,000 credits/day, 5-second resolution.
  At one poll per 10 seconds: 8,640 credits/day. Still exceeds.
- **Poll less frequently**: one poll per 30 seconds costs 2,880 credits/day. Fits
  within standard-user quota with margin.
- **Active feeder**: 8,000 credits/day. One poll per 10 seconds fits.

Default poll interval: **30 seconds**. Configurable. The fetcher enforces a minimum
interval of 10 seconds regardless of configuration.

On HTTP 429 (rate limited), the fetcher reads `X-Rate-Limit-Retry-After-Seconds` and
waits. On other errors, it logs and retries at the next poll interval. The fetcher never
crashes the process on transient API failures.

Authentication: OAuth2 client credentials flow. Client ID and secret are provided via
config. If not provided, the fetcher operates anonymously (tighter limits, 10-second
resolution).

### Normalizer

Converts the OpenSky JSON response into `[]Aircraft`. This is where unit conversions
happen:

- Altitude: meters to feet (`m * 3.28084`)
- Velocity: m/s to knots (`ms * 1.94384`)
- Callsign: trimmed of whitespace (OpenSky pads to 8 chars)

Barometric altitude is used (field index 7). Geometric altitude (index 13) is the
fallback if barometric is nil.

Aircraft with no ICAO24 are dropped (should not happen, but defensive).

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
every <poll_interval>:
    states, err = fetcher.Fetch()
    if err:
        log error
        // Do NOT pass empty list to tracker. A failed poll means
        // "we don't know" — not "the sky is empty." Active sightings
        // continue aging against their TTL naturally.
        continue
    aircraft = normalizer.Normalize(states)
    events = tracker.Process(aircraft)
    for event in events:
        renderer.Render(event)
```

The poll loop does not know about OpenSky, visibility math, or display formatting.

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

A YAML file. Example for a high-rise apartment in Seattle with southwest-facing windows:

```yaml
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

poll_interval: 30s

sighting_ttl: 60s

max_range_km: 60  # ~35 miles, naked-eye limit for commercial jets in clear weather
min_alt_ft: 150   # suppress aircraft at/near ground level
min_speed_kt: 50  # suppress taxiing aircraft reporting as airborne

opensky:
  # Omit for anonymous access
  client_id: ""
  client_secret: ""
```

Negative elevation minimum allows for aircraft below the observer's altitude (e.g.,
low approaches to SEA visible from an elevated vantage point).

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
    config/
      config.go        # YAML config parsing
    geo/
      geo.go           # bearing, elevation, distance calculations
    opensky/
      client.go        # HTTP client, auth, rate limiting
      types.go         # OpenSky API response types
      normalize.go     # OpenSky response -> []Aircraft
    model/
      aircraft.go      # Aircraft, Sighting, Event, Route types
    route/
      db.go            # Route database: load CSV, normalize callsign, lookup
      icao_to_iata.go  # ICAO airport code to IATA conversion
    tracker/
      tracker.go       # Visibility filtering, sighting state, event emission
    render/
      stdout.go        # STDOUT renderer
  data/
    routes/            # VRS standing data CSV files (git-tracked or downloaded)
  go.mod
  go.sum
```

## Scoped Out

- **LED matrix renderer**. The `Renderer` interface exists; a matrix implementation is
  additive.
- **ADS-B receiver input**. When this arrives, the internal model (`Aircraft`) stays the
  same. A new fetcher+normalizer pair replaces the OpenSky pair. The tracker and renderer
  are untouched.
- **"Will soon cross" prediction**. Trajectory prediction from heading and speed gives
  ~30-60 seconds of useful lookahead before altitude changes and ATC vectoring make it
  wrong. STAR/SID-aware prediction is a separate project. Deferred.
- **Live route lookup via API** (FlightAware, AeroAPI). The static VRS database covers
  scheduled commercial traffic. Real-time route lookup for GA/charter is a second data
  source dependency with its own rate limits and cost. Deferred.
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

**Building a formal data provider interface before the second provider exists.** The
ADS-B receiver's data shape is unknown until we have one. A premature abstraction will be
designed against imagination rather than reality. The `Aircraft` struct is the contract;
the interface emerges when the second implementation arrives.

**Python.** Mature geo libraries, but the geo math here is basic trig — no need for
shapely or pyproj. Go provides a single binary, clean cross-compilation for Raspberry Pi
(the likely LED matrix host), and a natural polling model with goroutines.

**Bearing-from-observer in the display.** The bearing (azimuth from observer) tells
you roughly where in the window to look. Dropped because the window view is narrow
enough that "it's somewhere out there" is sufficient, and the bearing is more useful
as an internal visibility computation than as a display field. Route information is
more valuable display real estate.
