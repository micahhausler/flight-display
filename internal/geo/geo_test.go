package geo

import (
	"math"
	"testing"
)

func TestBearing(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		wantMin, wantMax       float64 // acceptable range
	}{
		{"due north", 47.0, -122.0, 48.0, -122.0, 359, 1}, // ~0
		{"due east", 47.0, -122.0, 47.0, -121.0, 88, 92},
		{"due south", 48.0, -122.0, 47.0, -122.0, 179, 181},
		{"due west", 47.0, -121.0, 47.0, -122.0, 268, 272},
		{"southwest", 48.0, -121.0, 47.0, -122.0, 210, 230},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Bearing(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if tt.wantMin > tt.wantMax {
				// wraps around 0
				if got < tt.wantMin && got > tt.wantMax {
					t.Errorf("Bearing() = %v, want in [%v, 360) or [0, %v]", got, tt.wantMin, tt.wantMax)
				}
			} else {
				if got < tt.wantMin || got > tt.wantMax {
					t.Errorf("Bearing() = %v, want in [%v, %v]", got, tt.wantMin, tt.wantMax)
				}
			}
		})
	}
}

func TestGroundDistanceM(t *testing.T) {
	// Seattle to Portland: ~233 km
	d := GroundDistanceM(47.6062, -122.3321, 45.5152, -122.6784)
	if d < 230_000 || d > 236_000 {
		t.Errorf("Seattle to Portland = %v m, want ~233,000", d)
	}
}

func TestElevation(t *testing.T) {
	// Aircraft 10km away, 1000m above observer
	el := Elevation(10_000, 50, 1050)
	// atan(1000/10000) ≈ 5.71°
	if math.Abs(el-5.71) > 0.1 {
		t.Errorf("Elevation = %v, want ~5.71", el)
	}

	// Aircraft below observer
	el = Elevation(5_000, 100, 50)
	if el >= 0 {
		t.Errorf("Elevation = %v, want negative", el)
	}
}

func TestBBoxDeg(t *testing.T) {
	latMin, lonMin, latMax, lonMax := BBoxDeg(47.6, -122.3, 100_000)
	if latMax-latMin < 1.5 || latMax-latMin > 2.0 {
		t.Errorf("lat range = %v, want ~1.8", latMax-latMin)
	}
	if lonMax-lonMin < 2.0 || lonMax-lonMin > 3.5 {
		t.Errorf("lon range = %v, want ~2.6", lonMax-lonMin)
	}
	_ = latMin
	_ = lonMin
}
