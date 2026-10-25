package loadpoint

import (
	"fmt"
	"slices"
	"time"
	_ "time/tzdata"
)

// ChargingRestrictions defines local recurring intervals that block Smart charging.
type ChargingRestrictions struct {
	Enabled  bool              `json:"enabled"`
	Timezone string            `json:"timezone"`
	Rules    []RestrictionRule `json:"rules"`
}

// RestrictionRule matches its start date; an end before start spans midnight.
type RestrictionRule struct {
	Months   []int  `json:"months"`
	Weekdays []int  `json:"weekdays"` // ISO weekdays, Monday=1
	Start    string `json:"start"`
	End      string `json:"end"`
}

func (s ChargingRestrictions) Validate() error {
	if s.Timezone == "" {
		return fmt.Errorf("restriction timezone is required")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("invalid restriction timezone: %w", err)
	}
	if len(s.Rules) > 32 {
		return fmt.Errorf("at most 32 restriction rules are supported")
	}
	if s.Enabled && len(s.Rules) == 0 {
		return fmt.Errorf("at least one restriction rule is required")
	}
	for _, r := range s.Rules {
		for _, v := range r.Months {
			if v < 1 || v > 12 {
				return fmt.Errorf("invalid month: %d", v)
			}
		}
		for _, v := range r.Weekdays {
			if v < 1 || v > 7 {
				return fmt.Errorf("invalid weekday: %d", v)
			}
		}
		a, e1 := time.Parse("15:04", r.Start)
		b, e2 := time.Parse("15:04", r.End)
		if e1 != nil || e2 != nil || a.Format("15:04") != r.Start || b.Format("15:04") != r.End || a.Equal(b) {
			return fmt.Errorf("restriction requires distinct HH:MM start and end times")
		}
	}
	return nil
}

func (r RestrictionRule) matches(t time.Time) bool {
	day := int(t.Weekday())
	if day == 0 {
		day = 7
	}
	return (len(r.Months) == 0 || slices.Contains(r.Months, int(t.Month()))) &&
		(len(r.Weekdays) == 0 || slices.Contains(r.Weekdays, day))
}

// Blocked reports whether an interval includes now (start inclusive, end exclusive).
func (s ChargingRestrictions) Blocked(now time.Time) bool {
	if !s.Enabled {
		return false
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return true
	}
	return s.blockedLocal(now.In(loc))
}

func (s ChargingRestrictions) blockedLocal(now time.Time) bool {
	minute := now.Format("15:04")
	for _, r := range s.Rules {
		if r.Start < r.End {
			if r.matches(now) && minute >= r.Start && minute < r.End {
				return true
			}
		} else if r.matches(now) && minute >= r.Start || r.matches(now.AddDate(0, 0, -1)) && minute < r.End {
			return true
		}
	}
	return false
}

// NextTransition finds the next change of the combined rules, including DST changes.
func (s ChargingRestrictions) NextTransition(now time.Time) time.Time {
	if !s.Enabled {
		return time.Time{}
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}
	}
	// Scan real minutes so repeated and skipped local times follow the same rule as Blocked.
	// Called only on configuration changes and at transitions, not on every control cycle.
	blocked := s.Blocked(now)
	for t, end := now.Truncate(time.Minute).Add(time.Minute), now.AddDate(1, 0, 1); t.Before(end); t = t.Add(time.Minute) {
		if s.blockedLocal(t.In(loc)) != blocked {
			return t
		}
	}
	return time.Time{}
}
