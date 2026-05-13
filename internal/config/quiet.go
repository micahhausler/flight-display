package config

import (
	"fmt"
	"strings"
	"time"
)

// validDays is the set of recognized day names for quiet_hours keys.
var validDays = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

// dayKey returns the lowercase short day name for a time.Weekday.
func dayKey(wd time.Weekday) string {
	switch wd {
	case time.Sunday:
		return "sun"
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	}
	return ""
}

// prevDay returns the weekday before wd.
func prevDay(wd time.Weekday) time.Weekday {
	if wd == time.Sunday {
		return time.Saturday
	}
	return wd - 1
}

// parseHHMM parses a "HH:MM" string into hours and minutes.
func parseHHMM(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, 0, fmt.Errorf("invalid hour in %q: %w", s, err)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, 0, fmt.Errorf("invalid minute in %q: %w", s, err)
	}
	if h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("hour must be 0-23, got %d", h)
	}
	if m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("minute must be 0-59, got %d", m)
	}
	return h, m, nil
}

// timeOfDayMinutes returns the time-of-day in minutes since midnight.
func timeOfDayMinutes(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// validate checks that all QuietHours keys are valid day names and all
// time windows have valid HH:MM format.
func (qh QuietHours) validate() error {
	if qh == nil {
		return nil
	}
	for day, window := range qh {
		if _, ok := validDays[strings.ToLower(day)]; !ok {
			return fmt.Errorf("quiet_hours: invalid day %q (use sun/mon/tue/wed/thu/fri/sat)", day)
		}
		if _, _, err := parseHHMM(window.Start); err != nil {
			return fmt.Errorf("quiet_hours.%s.start: %w", day, err)
		}
		if _, _, err := parseHHMM(window.End); err != nil {
			return fmt.Errorf("quiet_hours.%s.end: %w", day, err)
		}
	}
	return nil
}

// InQuietHours reports whether the given time falls within a quiet hours window.
//
// The check considers two cases:
//  1. Today's window (if configured): does now fall within today's window?
//  2. Yesterday's window (if configured and crosses midnight): does now fall
//     within the "after midnight" portion of yesterday's window?
func InQuietHours(now time.Time, qh QuietHours) bool {
	if len(qh) == 0 {
		return false
	}

	todayMin := timeOfDayMinutes(now)

	// Check today's window
	if window, ok := qh[dayKey(now.Weekday())]; ok {
		startH, startM, _ := parseHHMM(window.Start)
		endH, endM, _ := parseHHMM(window.End)
		startMin := startH*60 + startM
		endMin := endH*60 + endM

		if startMin <= endMin {
			// Same-day window (e.g., 01:00 to 06:00)
			if todayMin >= startMin && todayMin < endMin {
				return true
			}
		} else {
			// Crosses midnight (e.g., 22:00 to 06:00): today's portion is >= start
			if todayMin >= startMin {
				return true
			}
		}
	}

	// Check yesterday's window — does it cross midnight and cover now?
	yesterday := prevDay(now.Weekday())
	if window, ok := qh[dayKey(yesterday)]; ok {
		startH, startM, _ := parseHHMM(window.Start)
		endH, endM, _ := parseHHMM(window.End)
		startMin := startH*60 + startM
		endMin := endH*60 + endM

		if startMin > endMin {
			// Yesterday's window crosses midnight; the "after midnight" portion is [00:00, end)
			if todayMin < endMin {
				return true
			}
		}
	}

	return false
}
