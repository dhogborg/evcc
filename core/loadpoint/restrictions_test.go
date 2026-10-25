package loadpoint

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func winterRestrictions() ChargingRestrictions {
	return ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []RestrictionRule{{Months: []int{1, 2, 3, 11, 12}, Weekdays: []int{1, 2, 3, 4, 5}, Start: "07:00", End: "20:00"}}}
}

func TestChargingRestrictionsBoundaries(t *testing.T) {
	s := winterRestrictions()
	require.NoError(t, s.Validate())
	for _, tc := range []struct {
		at      string
		blocked bool
	}{
		{"2026-11-02T06:59:59+01:00", false}, {"2026-11-02T07:00:00+01:00", true},
		{"2026-11-02T19:59:59+01:00", true}, {"2026-11-02T20:00:00+01:00", false},
		{"2026-11-07T12:00:00+01:00", false}, {"2026-04-01T12:00:00+02:00", false},
	} {
		t.Run(tc.at, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.at)
			require.NoError(t, err)
			assert.Equal(t, tc.blocked, s.Blocked(now))
		})
	}
	now, _ := time.Parse(time.RFC3339, "2026-11-02T19:59:59+01:00")
	assert.Equal(t, "2026-11-02T19:00:00Z", s.NextTransition(now).UTC().Format(time.RFC3339))
	s.Enabled = false
	assert.False(t, s.Blocked(now))
	assert.True(t, s.NextTransition(now).IsZero())
}

func TestChargingRestrictionsOvernightAndOverlap(t *testing.T) {
	s := ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []RestrictionRule{{Months: []int{3}, Weekdays: []int{2}, Start: "22:00", End: "08:00"}, {Start: "07:00", End: "09:00"}}}
	now, _ := time.Parse(time.RFC3339, "2026-04-01T06:30:00+02:00")
	assert.True(t, s.Blocked(now), "the overnight rule uses March 31, not April 1")
	assert.Equal(t, "2026-04-01T07:00:00Z", s.NextTransition(now).UTC().Format(time.RFC3339), "overlapping rules remain blocked until 09:00")
}

func TestChargingRestrictionsDST(t *testing.T) {
	s := ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []RestrictionRule{{Start: "02:15", End: "02:45"}}}
	now, _ := time.Parse(time.RFC3339, "2026-10-25T02:30:00+02:00")
	assert.True(t, s.Blocked(now))
	assert.Equal(t, now.Add(15*time.Minute), s.NextTransition(now))
	now = now.Add(20 * time.Minute)
	assert.False(t, s.Blocked(now))
	assert.Equal(t, "2026-10-25T01:15:00Z", s.NextTransition(now).UTC().Format(time.RFC3339))
	now, _ = time.Parse(time.RFC3339, "2026-03-29T01:59:00+01:00")
	assert.Equal(t, "2026-03-30T00:15:00Z", s.NextTransition(now).UTC().Format(time.RFC3339), "nonexistent local interval is skipped")
}

func TestChargingRestrictionsValidation(t *testing.T) {
	for _, mutate := range []func(*ChargingRestrictions){
		func(s *ChargingRestrictions) { s.Timezone = "Unknown/Zone" }, func(s *ChargingRestrictions) { s.Rules[0].Start = "24:00" },
		func(s *ChargingRestrictions) { s.Rules[0].End = s.Rules[0].Start }, func(s *ChargingRestrictions) { s.Rules[0].Months = []int{13} },
		func(s *ChargingRestrictions) { s.Rules[0].Weekdays = []int{0} }, func(s *ChargingRestrictions) { s.Rules = nil },
	} {
		s := winterRestrictions()
		mutate(&s)
		assert.Error(t, s.Validate())
	}
}
