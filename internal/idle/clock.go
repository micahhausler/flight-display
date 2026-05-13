package idle

import (
	"time"

	"github.com/micahhausler/flight-display/internal/model"
)

// ClockProvider displays the current local time.
type ClockProvider struct{}

func NewClockProvider() *ClockProvider {
	return &ClockProvider{}
}

func (p *ClockProvider) Name() string { return "clock" }

func (p *ClockProvider) Current() (model.IdleInfo, bool) {
	return model.IdleInfo{
		Icon:    model.IconClock,
		Primary: time.Now().Format("3:04 PM"),
	}, true
}

// DateProvider displays the current date.
type DateProvider struct{}

func NewDateProvider() *DateProvider {
	return &DateProvider{}
}

func (p *DateProvider) Name() string { return "date" }

func (p *DateProvider) Current() (model.IdleInfo, bool) {
	return model.IdleInfo{
		Icon:    model.IconDate,
		Primary: time.Now().Format("Mon Jan 2"),
	}, true
}
