package route

import (
	"encoding/csv"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/micahhausler/flight-display/internal/model"
)

// DB is an in-memory route database loaded from VRS standing data CSV files.
type DB struct {
	routes map[string]model.Route // key: normalized callsign (e.g. "DAL1921")
}

// callsignRegex splits a callsign into code and number per VRS normalization rules.
var callsignRegex = regexp.MustCompile(`^([A-Z]{2,3}|[A-Z][0-9]|[0-9][A-Z])(\d[A-Z0-9]*)`)

// NormalizeCallsign normalizes a callsign per VRS rules:
// - Split into code + number
// - Strip leading zeros from number (keep at least one digit)
// - Uppercase
func NormalizeCallsign(callsign string) string {
	cs := strings.ToUpper(strings.TrimSpace(callsign))
	m := callsignRegex.FindStringSubmatch(cs)
	if m == nil {
		return cs
	}
	code := m[1]
	number := m[2]

	// Strip leading zeros but keep at least one character
	stripped := strings.TrimLeft(number, "0")
	if stripped == "" || (stripped[0] < '0' || stripped[0] > '9') {
		stripped = "0" + stripped
	}

	return code + stripped
}

// LoadDB loads route CSV files from the given directory tree.
// The directory structure follows VRS standing-data format:
// routes/<first-char>/<CODE>-all.csv or <CODE>-<digit>.csv
func LoadDB(dir string) (*DB, error) {
	db := &DB{
		routes: make(map[string]model.Route),
	}

	count := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".csv") {
			return nil
		}

		n, loadErr := db.loadCSV(path)
		if loadErr != nil {
			log.Printf("warning: skipping %s: %v", path, loadErr)
			return nil // don't fail on individual bad files
		}
		count += n
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking route directory: %w", err)
	}

	log.Printf("Loaded %d routes from %s", count, dir)
	return db, nil
}

func (db *DB) loadCSV(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.LazyQuotes = true

	records, err := r.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("parsing CSV: %w", err)
	}

	count := 0
	for i, rec := range records {
		if i == 0 {
			continue // skip header
		}
		if len(rec) < 5 {
			continue
		}

		callsign := rec[0]
		airportCodes := rec[4]
		if callsign == "" || airportCodes == "" {
			continue
		}

		icaoCodes := strings.Split(airportCodes, "-")
		iataCodes := make([]string, len(icaoCodes))
		for j, ic := range icaoCodes {
			iataCodes[j] = ICAOToIATA(ic)
		}

		db.routes[callsign] = model.Route{Airports: iataCodes}
		count++
	}

	return count, nil
}

// Lookup returns the route for a callsign, or nil if not found.
// The callsign is normalized before lookup.
func (db *DB) Lookup(callsign string) *model.Route {
	if db == nil {
		return nil
	}
	normalized := NormalizeCallsign(callsign)
	if r, ok := db.routes[normalized]; ok {
		return &r
	}
	return nil
}
