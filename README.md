# flight-display

An ambient display that shows aircraft visible from your window. You look outside, see a plane, and glance at the display to know what it is.

```
+ DAL1714  SEA-SLC   1175ft
+ ASA404   SEA-SFO   2600ft
+ QXE2037  SEA-PDX   6425ft
+ N64SV              9750ft
```

## How it works

You define your observer position (lat/lon/altitude) and the angular range of your window (azimuth and elevation bounds). The system polls [OpenSky Network](https://opensky-network.org/) for aircraft state vectors, filters them by:

1. **Aperture** — is the aircraft's bearing and elevation from you within your window's view?
2. **Slant range** — is it close enough to actually see? (default 60 km)
3. **Altitude and speed** — is it actually flying, not taxiing or parked?

When an aircraft enters your view, it prints a line. When it leaves, it prints another. Route information (e.g., SEA-SFO) comes from the [VRS standing data](https://github.com/vradarserver/standing-data) CSV database, loaded at startup.

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

## Configuration

Copy the example config and edit it for your location:

```sh
cp config.example.yaml config.yaml
```

```yaml
observer:
  lat: 47.6115
  lon: -122.3470
  alt_msl_meters: 55  # ground elevation + floor height

aperture:
  rects:
    - az_min: 160     # start of view (degrees, clockwise from north)
      az_max: 290     # end of view
      el_min: -2      # minimum elevation angle
      el_max: 30      # maximum elevation angle

poll_interval: 30s
sighting_ttl: 60s
max_range_km: 60      # ~35 miles, naked-eye limit
min_alt_ft: 150       # suppress ground-level aircraft
min_speed_kt: 50      # suppress taxiing aircraft

# routes_dir: data/routes   # path to VRS standing-data route CSVs

opensky:
  client_id: ""       # optional, for higher API rate limits
  client_secret: ""
```

### Finding your aperture

The aperture defines what your window can see as compass bearings (azimuth) and angles above the horizon (elevation).

- **Azimuth**: Use a compass app on your phone. Stand at the window and note the bearing to the left edge and right edge of your view. `az_min` is the left edge, `az_max` is the right edge, going clockwise.
- **Elevation**: 0° is the horizon, 90° is straight up. Most windows span roughly -2° to 30°.
- **Multiple rectangles**: If your view wraps around a corner or has obstructions, use multiple rects.

Start with generous ranges and narrow them based on which displayed flights you can actually see.

### Route enrichment

To display route information (origin/destination airports), download the [VRS standing data](https://github.com/vradarserver/standing-data) route CSVs:

```sh
git clone https://github.com/vradarserver/standing-data.git data/standing-data
```

Then set `routes_dir` in your config:

```yaml
routes_dir: data/standing-data/routes/schema-01
```

This covers most scheduled commercial traffic. GA, military, and charter flights will show callsign and altitude only.

## Usage

```sh
./flight-display -config config.yaml
```

Output:

```
+ DAL1714  SEA-SLC   1175ft    # Delta 1714 entered your view
+ ASA404   SEA-SFO   2600ft    # Alaska 404 entered
- DAL1714  SEA-SLC             # Delta 1714 left your view
+ N64SV              9750ft    # GA aircraft, no route in database
```

### OpenSky API limits

Anonymous access: 400 credits/day, 10-second state resolution. At the default 30-second poll interval, this costs ~2,880 credits/day — you'll need a free [OpenSky account](https://opensky-network.org/index.php/login) for authenticated access (4,000 credits/day). Active ADS-B feeders get 8,000 credits/day.

## License

MIT
