package route

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookup_ExactMatch(t *testing.T) {
	dir := setupTestRoutes(t)
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}

	r := db.Lookup("DAL1921")
	if r == nil {
		t.Fatal("expected route for DAL1921, got nil")
	}
	if r.Airports[0] != "SEA" || r.Airports[1] != "BNA" {
		t.Errorf("expected SEA-BNA, got %v", r.Airports)
	}
}

func TestLookup_SuffixStrip(t *testing.T) {
	dir := setupTestRoutes(t)
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}

	// SKW112J is not in the DB, but SKW112 is — suffix stripping should find it
	r := db.Lookup("SKW112J")
	if r == nil {
		t.Fatal("expected route for SKW112J via suffix stripping, got nil")
	}
	if r.Airports[0] != "GEG" || r.Airports[1] != "SEA" {
		t.Errorf("expected GEG-SEA, got %v", r.Airports)
	}
}

func TestLookup_SuffixStrip_DoubleLetter(t *testing.T) {
	dir := setupTestRoutes(t)
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}

	// SKW368P not in DB, SKW368 is — should strip P
	r := db.Lookup("SKW368P")
	if r == nil {
		t.Fatal("expected route for SKW368P via suffix stripping, got nil")
	}
	if r.Airports[0] != "SEA" || r.Airports[1] != "PDX" {
		t.Errorf("expected SEA-PDX, got %v", r.Airports)
	}
}

func TestLookup_NoMatch(t *testing.T) {
	dir := setupTestRoutes(t)
	db, err := LoadDB(dir)
	if err != nil {
		t.Fatalf("LoadDB: %v", err)
	}

	r := db.Lookup("ZZZZZ")
	if r != nil {
		t.Errorf("expected nil for unknown callsign, got %v", r)
	}
}

func TestNormalizeCallsign(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"DAL1921", "DAL1921"},
		{"DAL0001", "DAL1"},
		{"SKW112J", "SKW112J"},
		{"  dal1921  ", "DAL1921"},
		{"N12345", "N12345"}, // not an airline callsign, returned as-is
	}
	for _, tt := range tests {
		got := NormalizeCallsign(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeCallsign(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// setupTestRoutes creates a temp directory with minimal CSV route files for testing.
func setupTestRoutes(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	dDir := filepath.Join(dir, "D")
	sDir := filepath.Join(dir, "S")
	os.MkdirAll(dDir, 0o755)
	os.MkdirAll(sDir, 0o755)

	// DAL routes
	dalCSV := `Callsign,Code,Number,AirlineCode,AirportCodes
DAL1921,DAL,1921,DAL,KSEA-KBNA
DAL1714,DAL,1714,DAL,KSEA-KSLC
`
	os.WriteFile(filepath.Join(dDir, "DAL-all.csv"), []byte(dalCSV), 0o644)

	// SKW routes — note: SKW112 exists but SKW112J does not
	skwCSV := `Callsign,Code,Number,AirlineCode,AirportCodes
SKW112,SKW,112,SKW,KGEG-KSEA
SKW112C,SKW,112C,SKW,KGEG-KLAX
SKW368,SKW,368,SKW,KSEA-KPDX
`
	os.WriteFile(filepath.Join(sDir, "SKW-all.csv"), []byte(skwCSV), 0o644)

	return dir
}
