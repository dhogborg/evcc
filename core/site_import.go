package core

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/keys"
	"github.com/evcc-io/evcc/core/types"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/evcc-io/evcc/util/grid"
)

// importCheckpoint keeps the import register across restarts; exports never reduce Used.
type importCheckpoint struct {
	Meter  string    `json:"meter"`
	Hour   time.Time `json:"hour"`
	At     time.Time `json:"at"`
	Energy float64   `json:"energy"`
	Used   float64   `json:"used"`
	Valid  bool      `json:"valid"`
}

type hourlyImportControl struct {
	sync.Mutex
	limit        float64
	checkpoint   importCheckpoint
	fresh        bool
	gridPower    float64
	chargePower  float64
	reservations map[*Loadpoint]float64
}

type hourlyImportStatus struct {
	Used         float64   `json:"used"`
	Remaining    float64   `json:"remaining"`
	AllowedPower float64   `json:"allowedPower"` // W
	Reset        time.Time `json:"reset"`
	Status       string    `json:"status"`
}

func importHour(t time.Time) time.Time {
	loc, _ := time.LoadLocation("Europe/Stockholm")
	t = t.In(loc)
	// Subtract elapsed minutes instead of reconstructing a repeated DST hour.
	return t.Add(-time.Duration(t.Minute())*time.Minute - time.Duration(t.Second())*time.Second - time.Duration(t.Nanosecond()))
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// observe accounts the entire boundary-spanning delta to the new hour conservatively.
func (c *importCheckpoint) observe(now time.Time, meter string, energy float64) {
	hour := importHour(now)
	sameMeter := c.Meter == meter
	ordered := now.After(c.At)
	delta := energy - c.Energy
	if !c.Hour.Equal(hour) {
		valid := sameMeter && ordered && !c.At.Before(hour.Add(-2*time.Minute)) && c.At.Before(hour) && delta >= 0
		*c = importCheckpoint{Meter: meter, Hour: hour, At: now, Energy: energy, Valid: valid}
		if valid {
			c.Used = delta
		}
		return
	}
	if !sameMeter || !ordered || delta < 0 {
		c.Valid = false
	} else {
		c.Used += delta
	}
	c.Meter, c.At, c.Energy = meter, now, energy
}

func (c *hourlyImportControl) status(now time.Time) hourlyImportStatus {
	s := hourlyImportStatus{Used: c.checkpoint.Used, Reset: importHour(now).Add(time.Hour), Status: "disabled"}
	if c.limit == 0 {
		return s
	}
	s.Remaining = max(0, c.limit-s.Used)
	if !c.fresh || !c.checkpoint.Valid || !c.checkpoint.Hour.Equal(importHour(now)) || now.Sub(c.checkpoint.At) > 2*time.Minute {
		s.Status = "meterUnavailable"
		if c.fresh && !c.checkpoint.Valid {
			s.Status = "awaitingHour"
		}
		return s
	}
	// Reserve at least 50 Wh and two minutes of measured import for measurement and actuation delay.
	reserve := max(0.05, max(0, c.gridPower)*2/60/1000)
	s.AllowedPower = max(0, s.Remaining-reserve) / s.Reset.Sub(now).Hours() * 1000
	s.Status = "active"
	if s.Remaining == 0 {
		s.Status = "exhausted"
	}
	return s
}

// GetHourlyImportLimit returns the site-wide clock-hour import budget in kWh (0 disables it).
func (site *Site) GetHourlyImportLimit() float64 {
	c := &site.importControl
	c.Lock()
	defer c.Unlock()
	return c.limit
}

// SetHourlyImportLimit changes the budget without resetting the hour's consumption.
func (site *Site) SetHourlyImportLimit(limit float64) error {
	if !finite(limit) || limit < 0 || limit > 1e6 {
		return fmt.Errorf("invalid hourly import limit: %g", limit)
	}
	if limit > 0 && (site.gridMeter == nil || !api.HasCap[api.MeterEnergy](site.gridMeter.Instance())) {
		return fmt.Errorf("hourly import control requires a grid meter with cumulative import energy")
	}
	c := &site.importControl
	c.Lock()
	c.limit = limit
	c.Unlock()
	settings.SetFloat(keys.HourlyImportLimit, limit)
	site.publish(keys.HourlyImportLimit, limit)
	for _, lp := range site.activeLoadpoints() {
		lp.requestUpdate()
	}
	return nil
}

func (site *Site) restoreHourlyImport() {
	c := &site.importControl
	_ = settings.Json(keys.HourlyImportCheckpoint, &c.checkpoint)
	if limit, err := settings.Float(keys.HourlyImportLimit); err == nil && finite(limit) && limit >= 0 {
		c.limit = limit
	}
}

func (site *Site) recordHourlyImport(now time.Time, m types.Measurement) {
	c := &site.importControl
	c.Lock()
	defer c.Unlock()
	c.fresh = m.Energy != nil && finite(*m.Energy) && *m.Energy >= 0 && finite(m.Power)
	if c.fresh {
		c.gridPower = m.Power
		c.checkpoint.observe(now, m.Name, *m.Energy)
		if err := settings.SetJson(keys.HourlyImportCheckpoint, c.checkpoint); err != nil {
			site.log.ERROR.Println(err)
			c.fresh = false
		}
	}
}

func (site *Site) invalidateHourlyImport() {
	c := &site.importControl
	c.Lock()
	c.fresh = false
	status := c.status(time.Now())
	c.Unlock()
	site.publish(keys.HourlyImport, status)
}

func (site *Site) prepareHourlyImport(totalChargePower float64) {
	c := &site.importControl
	c.Lock()
	c.chargePower = totalChargePower
	if c.reservations == nil {
		c.reservations = make(map[*Loadpoint]float64)
	}
	if site.gridMeter == nil {
		c.fresh = false
	}
	for _, lp := range site.activeLoadpoints() {
		// Reserve measured power until a charger actually responds to a reduction.
		c.reservations[lp] = max(lp.chargePower, grid.CurrentToPower(lp.offeredCurrent, lp.ActivePhases()))
	}
	s := c.status(time.Now())
	c.Unlock()
	site.publish(keys.HourlyImport, s)
}

// limitImportPower reserves aggregate headroom so multiple chargers cannot claim it twice.
func (site *Site) limitImportPower(lp *Loadpoint, requested float64) float64 {
	c := &site.importControl
	c.Lock()
	defer c.Unlock()
	if c.limit == 0 {
		return requested
	}
	if c.reservations == nil {
		c.reservations = make(map[*Loadpoint]float64)
	}
	s := c.status(time.Now())
	allowed := 0.0
	if s.Status == "active" || s.Status == "exhausted" {
		available := max(0, s.AllowedPower-c.gridPower+c.chargePower)
		for other, power := range c.reservations {
			if other != lp {
				available -= power
			}
		}
		allowed = min(requested, max(0, available))
	}
	c.reservations[lp] = max(lp.chargePower, allowed)
	return allowed
}

// enforceImportAndSchedules sheds all affected loadpoints each cycle, not just the selected one.
func (site *Site) enforceImportAndSchedules() {
	for _, lp := range site.activeLoadpoints() {
		lp.publishChargingRestriction()
		if site.GetHourlyImportLimit() > 0 || lp.scheduleBlocked() {
			if err := lp.setLimit(lp.offeredCurrent); err != nil {
				lp.log.ERROR.Println(err)
			}
		}
	}
}
