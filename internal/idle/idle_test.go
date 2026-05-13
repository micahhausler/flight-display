package idle

import (
	"testing"

	"github.com/micahhausler/flight-display/internal/model"
)

// stubProvider is a test helper that returns a fixed value or reports unavailable.
type stubProvider struct {
	name  string
	info  model.IdleInfo
	avail bool
}

func (s *stubProvider) Name() string                    { return s.name }
func (s *stubProvider) Current() (model.IdleInfo, bool) { return s.info, s.avail }

func TestRotatorNextCyclesProviders(t *testing.T) {
	providers := []Provider{
		&stubProvider{name: "a", info: model.IdleInfo{Icon: model.IconClock, Primary: "A"}, avail: true},
		&stubProvider{name: "b", info: model.IdleInfo{Icon: model.IconDate, Primary: "B"}, avail: true},
		&stubProvider{name: "c", info: model.IdleInfo{Icon: model.IconTemperature, Primary: "C"}, avail: true},
	}
	r := NewRotator(providers)
	r.Reset()

	// Should cycle through a, b, c, a, b, c, ...
	expected := []string{"A", "B", "C", "A", "B", "C"}
	for i, want := range expected {
		ev, ok := r.Next()
		if !ok {
			t.Fatalf("Next() returned !ok at iteration %d", i)
		}
		if ev.IdleInfo.Primary != want {
			t.Errorf("iteration %d: got %q, want %q", i, ev.IdleInfo.Primary, want)
		}
		if ev.Kind != model.Idle {
			t.Errorf("iteration %d: got kind %d, want Idle (%d)", i, ev.Kind, model.Idle)
		}
	}
}

func TestRotatorSkipsUnavailableProviders(t *testing.T) {
	providers := []Provider{
		&stubProvider{name: "a", info: model.IdleInfo{Primary: "A"}, avail: true},
		&stubProvider{name: "b", info: model.IdleInfo{Primary: "B"}, avail: false}, // unavailable
		&stubProvider{name: "c", info: model.IdleInfo{Primary: "C"}, avail: true},
	}
	r := NewRotator(providers)
	r.Reset()

	// Should skip "b" and go a, c, a, c, ...
	expected := []string{"A", "C", "A", "C"}
	for i, want := range expected {
		ev, ok := r.Next()
		if !ok {
			t.Fatalf("Next() returned !ok at iteration %d", i)
		}
		if ev.IdleInfo.Primary != want {
			t.Errorf("iteration %d: got %q, want %q", i, ev.IdleInfo.Primary, want)
		}
	}
}

func TestRotatorAllUnavailableReturnsFalse(t *testing.T) {
	providers := []Provider{
		&stubProvider{name: "a", avail: false},
		&stubProvider{name: "b", avail: false},
	}
	r := NewRotator(providers)
	r.Reset()

	_, ok := r.Next()
	if ok {
		t.Error("Next() should return false when all providers are unavailable")
	}
}

func TestRotatorEmptyProviders(t *testing.T) {
	r := NewRotator(nil)
	_, ok := r.Next()
	if ok {
		t.Error("Next() should return false with no providers")
	}
}

func TestRotatorReset(t *testing.T) {
	providers := []Provider{
		&stubProvider{name: "a", info: model.IdleInfo{Primary: "A"}, avail: true},
		&stubProvider{name: "b", info: model.IdleInfo{Primary: "B"}, avail: true},
		&stubProvider{name: "c", info: model.IdleInfo{Primary: "C"}, avail: true},
	}
	r := NewRotator(providers)

	// Advance past "a"
	r.Reset()
	r.Next() // a
	r.Next() // b

	// Reset should make next call return "a" again
	r.Reset()
	ev, ok := r.Next()
	if !ok {
		t.Fatal("Next() returned !ok after reset")
	}
	if ev.IdleInfo.Primary != "A" {
		t.Errorf("after reset, got %q, want %q", ev.IdleInfo.Primary, "A")
	}
}

func TestClockProviderAlwaysAvailable(t *testing.T) {
	p := NewClockProvider()
	if p.Name() != "clock" {
		t.Errorf("Name() = %q, want %q", p.Name(), "clock")
	}
	info, ok := p.Current()
	if !ok {
		t.Fatal("ClockProvider.Current() returned !ok")
	}
	if info.Icon != model.IconClock {
		t.Errorf("Icon = %d, want IconClock (%d)", info.Icon, model.IconClock)
	}
	if info.Primary == "" {
		t.Error("Primary should not be empty")
	}
}

func TestDateProviderAlwaysAvailable(t *testing.T) {
	p := NewDateProvider()
	if p.Name() != "date" {
		t.Errorf("Name() = %q, want %q", p.Name(), "date")
	}
	info, ok := p.Current()
	if !ok {
		t.Fatal("DateProvider.Current() returned !ok")
	}
	if info.Icon != model.IconDate {
		t.Errorf("Icon = %d, want IconDate (%d)", info.Icon, model.IconDate)
	}
	if info.Primary == "" {
		t.Error("Primary should not be empty")
	}
}
