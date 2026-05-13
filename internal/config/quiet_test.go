package config

import (
	"testing"
	"time"
)

func makeTime(weekday time.Weekday, hour, minute int) time.Time {
	// Use a known date: 2024-01-01 is a Monday.
	// Offset by weekday to get the desired day.
	// Monday = 1, so day 1 is Monday in this reference week.
	baseMonday := time.Date(2024, 1, 1, hour, minute, 0, 0, time.Local)
	dayOffset := int(weekday) - int(time.Monday)
	if dayOffset < 0 {
		dayOffset += 7
	}
	return baseMonday.AddDate(0, 0, dayOffset)
}

func TestInQuietHours(t *testing.T) {
	tests := []struct {
		name    string
		qh      QuietHours
		now     time.Time
		want    bool
	}{
		{
			name: "nil quiet hours",
			qh:   nil,
			now:  makeTime(time.Monday, 23, 0),
			want: false,
		},
		{
			name: "empty quiet hours",
			qh:   QuietHours{},
			now:  makeTime(time.Monday, 23, 0),
			want: false,
		},
		{
			name: "within same-day window",
			qh:   QuietHours{"mon": {Start: "01:00", End: "06:00"}},
			now:  makeTime(time.Monday, 3, 30),
			want: true,
		},
		{
			name: "outside same-day window (before)",
			qh:   QuietHours{"mon": {Start: "01:00", End: "06:00"}},
			now:  makeTime(time.Monday, 0, 30),
			want: false,
		},
		{
			name: "outside same-day window (after)",
			qh:   QuietHours{"mon": {Start: "01:00", End: "06:00"}},
			now:  makeTime(time.Monday, 6, 30),
			want: false,
		},
		{
			name: "at start boundary (inclusive)",
			qh:   QuietHours{"mon": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Monday, 22, 0),
			want: true,
		},
		{
			name: "at end boundary (exclusive)",
			qh:   QuietHours{"mon": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Tuesday, 6, 0),
			want: false,
		},
		{
			name: "cross-midnight: before midnight on start day",
			qh:   QuietHours{"mon": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Monday, 23, 30),
			want: true,
		},
		{
			name: "cross-midnight: after midnight on next day",
			qh:   QuietHours{"mon": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Tuesday, 3, 0),
			want: true,
		},
		{
			name: "cross-midnight: after end on next day",
			qh:   QuietHours{"mon": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Tuesday, 7, 0),
			want: false,
		},
		{
			name: "day not configured",
			qh:   QuietHours{"fri": {Start: "22:00", End: "06:00"}},
			now:  makeTime(time.Monday, 23, 0),
			want: false,
		},
		{
			name: "sunday cross-midnight into monday",
			qh:   QuietHours{"sun": {Start: "22:00", End: "07:00"}},
			now:  makeTime(time.Monday, 5, 0),
			want: true,
		},
		{
			name: "saturday cross-midnight into sunday",
			qh:   QuietHours{"sat": {Start: "23:00", End: "08:00"}},
			now:  makeTime(time.Sunday, 2, 0),
			want: true,
		},
		{
			name: "saturday cross-midnight: before window starts",
			qh:   QuietHours{"sat": {Start: "23:00", End: "08:00"}},
			now:  makeTime(time.Saturday, 20, 0),
			want: false,
		},
		{
			name: "multiple days: hit tuesday",
			qh: QuietHours{
				"mon": {Start: "22:00", End: "06:00"},
				"tue": {Start: "22:00", End: "06:00"},
			},
			now:  makeTime(time.Tuesday, 23, 0),
			want: true,
		},
		{
			name: "multiple days: hit mon cross-midnight on tue morning",
			qh: QuietHours{
				"mon": {Start: "22:00", End: "06:00"},
				"tue": {Start: "22:00", End: "06:00"},
			},
			now:  makeTime(time.Tuesday, 4, 0),
			want: true,
		},
		{
			name: "same-day window: end boundary check",
			qh:   QuietHours{"wed": {Start: "00:00", End: "06:30"}},
			now:  makeTime(time.Wednesday, 0, 0),
			want: true,
		},
		{
			name: "same-day window: 06:29 is in",
			qh:   QuietHours{"wed": {Start: "00:00", End: "06:30"}},
			now:  makeTime(time.Wednesday, 6, 29),
			want: true,
		},
		{
			name: "same-day window: 06:30 is out",
			qh:   QuietHours{"wed": {Start: "00:00", End: "06:30"}},
			now:  makeTime(time.Wednesday, 6, 30),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InQuietHours(tt.now, tt.qh)
			if got != tt.want {
				t.Errorf("InQuietHours(%v, %v) = %v, want %v", tt.now.Format("Mon 15:04"), tt.qh, got, tt.want)
			}
		})
	}
}

func TestQuietHoursValidate(t *testing.T) {
	tests := []struct {
		name    string
		qh      QuietHours
		wantErr bool
	}{
		{name: "nil is valid", qh: nil, wantErr: false},
		{name: "empty is valid", qh: QuietHours{}, wantErr: false},
		{name: "valid day", qh: QuietHours{"mon": {Start: "22:00", End: "06:00"}}, wantErr: false},
		{name: "invalid day", qh: QuietHours{"monday": {Start: "22:00", End: "06:00"}}, wantErr: true},
		{name: "invalid start format", qh: QuietHours{"mon": {Start: "25:00", End: "06:00"}}, wantErr: true},
		{name: "invalid end format", qh: QuietHours{"mon": {Start: "22:00", End: "6"}}, wantErr: true},
		{name: "bad minute", qh: QuietHours{"mon": {Start: "22:61", End: "06:00"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.qh.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
