package core

import (
	"testing"
	"time"

	evbus "github.com/asaskevich/EventBus"
	"github.com/benbjohnson/clock"
	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/loadpoint"
	"github.com/evcc-io/evcc/core/settings"
	"github.com/evcc-io/evcc/util"
	"github.com/evcc-io/evcc/util/encode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestScheduleSmartOnlyAndPersistence(t *testing.T) {
	store := settings.NewMemorySettings()
	lp := NewLoadpoint(util.NewLogger("test"), store)
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	lp.clock = mockClock
	s := loadpoint.ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []loadpoint.RestrictionRule{{Start: "07:00", End: "20:00"}}}
	require.NoError(t, lp.SetChargingRestrictions(s))
	lp.mode = api.ModeSmart
	assert.True(t, lp.scheduleBlocked())
	lp.mode = api.ModeNow
	assert.False(t, lp.scheduleBlocked(), "Fast bypasses recurring restrictions")
	restored := NewLoadpoint(util.NewLogger("restored"), store)
	restored.restoreChargingRestrictions()
	assert.Equal(t, s, restored.GetChargingRestrictions())
	copy := lp.GetChargingRestrictions()
	copy.Rules[0].Start = "00:00"
	assert.Equal(t, "07:00", lp.GetChargingRestrictions().Rules[0].Start)
}

func TestScheduleFinalLimitCannotBeOverridden(t *testing.T) {
	ctrl := gomock.NewController(t)
	charger := api.NewMockCharger(ctrl)
	charger.EXPECT().Enable(false).Return(nil)
	lp := NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
	lp.wakeUpTimer = NewTimer()
	lp.charger = charger
	lp.bus = evbus.New()
	lp.enabled = true
	lp.offeredCurrent = 16
	lp.phasesConfigured = 1
	lp.phases = 1
	lp.mode = api.ModeSmart
	mockClock := clock.NewMock()
	mockClock.Set(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	lp.clock = mockClock
	require.NoError(t, lp.SetChargingRestrictions(loadpoint.ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []loadpoint.RestrictionRule{{Start: "07:00", End: "20:00"}}}))
	require.NoError(t, lp.setLimit(16), "even welcome/minimum/plan current is clamped to zero")
	assert.False(t, lp.enabled)
}

func TestScheduleBoundaryRequestsUpdate(t *testing.T) {
	lp := NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
	mc := clock.NewMock()
	mc.Set(time.Date(2026, 11, 2, 5, 59, 0, 0, time.UTC))
	lp.clock = mc
	updates := make(chan *Loadpoint, 1)
	lp.lpChan = updates
	require.NoError(t, lp.SetChargingRestrictions(loadpoint.ChargingRestrictions{Enabled: true, Timezone: "Europe/Stockholm", Rules: []loadpoint.RestrictionRule{{Start: "07:00", End: "20:00"}}}))
	<-updates
	lp.publishChargingRestriction()
	mc.Add(time.Minute)
	select {
	case <-updates:
	default:
		t.Fatal("restriction boundary did not request a control update")
	}
}

func TestFinalChargingConstraintsByMode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     api.ChargeMode
		schedule bool
		budget   bool
		fresh    bool
		used     float64
		disabled bool
	}{
		{"Fast bypasses schedule", api.ModeNow, true, false, false, 0, false},
		{"Smart respects schedule", api.ModeSmart, true, false, false, 0, true},
		{"Fast respects missing import readings", api.ModeNow, true, true, false, 0, true},
		{"Fast respects exhausted budget", api.ModeNow, false, true, true, 14, true},
		{"Smart respects exhausted budget", api.ModeSmart, false, true, true, 14, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			charger := api.NewMockCharger(ctrl)
			if tc.disabled {
				charger.EXPECT().Enable(false).Return(nil)
			}
			lp := NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
			lp.charger = charger
			lp.bus = evbus.New()
			lp.wakeUpTimer = NewTimer()
			lp.enabled = true
			lp.offeredCurrent = 16
			lp.phases = 1
			lp.phasesConfigured = 1
			lp.mode = tc.mode
			mc := clock.NewMock()
			mc.Set(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
			lp.clock = mc
			lp.restrictions = loadpoint.ChargingRestrictions{Enabled: tc.schedule, Timezone: "Europe/Stockholm", Rules: []loadpoint.RestrictionRule{{Start: "07:00", End: "20:00"}}}
			site := new(Site)
			if tc.budget {
				site.importControl.limit = 14
				site.importControl.fresh = tc.fresh
				now := time.Now()
				site.importControl.checkpoint = importCheckpoint{Hour: importHour(now), At: now, Used: tc.used, Valid: true}
			}
			lp.site = site
			require.NoError(t, lp.setLimit(16))
			assert.Equal(t, !tc.disabled, lp.enabled)
		})
	}
}

func TestImportAllowanceConvertsPowerToCurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	charger := api.NewMockCharger(ctrl)
	charger.EXPECT().MaxCurrent(int64(10)).Return(nil)
	lp := NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
	lp.charger = charger
	lp.bus = evbus.New()
	lp.enabled = true
	lp.offeredCurrent = 16
	lp.phases = 3
	lp.phasesConfigured = 3
	lp.mode = api.ModeNow
	lp.chargePower = 3000
	site := new(Site)
	lp.site = site
	now := time.Now()
	remainingHours := importHour(now).Add(time.Hour).Sub(now).Hours()
	site.importControl.limit = 14
	site.importControl.fresh = true
	site.importControl.gridPower = 6000
	site.importControl.chargePower = 3000
	// 9.9 kW grid allowance minus 3 kW household demand leaves 6.9 kW, or 10 A on 3 phases.
	site.importControl.checkpoint = importCheckpoint{Hour: importHour(now), At: now, Used: 14 - .2 - 9.9*remainingHours, Valid: true}
	require.NoError(t, lp.setLimit(16))
	assert.Equal(t, 10.0, lp.offeredCurrent)
	assert.True(t, lp.enabled)
}

func TestScheduleNoTransitionStateEncoding(t *testing.T) {
	lp := NewLoadpoint(util.NewLogger("test"), settings.NewMemorySettings())
	values := make(chan util.Param, 2)
	lp.uiChan = values
	lp.publishChargingRestriction()
	for range 2 {
		param := <-values
		require.NotPanics(t, func() { encode.NewEncoder().Encode(param.Val) }, "empty schedule timestamps must serialize safely")
	}
}
