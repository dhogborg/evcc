package core

import (
	"encoding/json"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/keys"
	"github.com/evcc-io/evcc/core/loadpoint"
	"github.com/evcc-io/evcc/util/grid"
)

// GetChargingRestrictions returns an independent copy of the recurring restrictions.
func (lp *Loadpoint) GetChargingRestrictions() loadpoint.ChargingRestrictions {
	lp.RLock()
	defer lp.RUnlock()
	// The rules contain slices which must not escape the loadpoint lock.
	data, _ := json.Marshal(lp.restrictions)
	var res loadpoint.ChargingRestrictions
	_ = json.Unmarshal(data, &res)
	if res.Timezone == "" {
		res.Timezone = "Europe/Stockholm"
	}
	if res.Rules == nil {
		res.Rules = []loadpoint.RestrictionRule{}
	}
	return res
}

// SetChargingRestrictions validates and persists the loadpoint's Smart-mode restrictions.
func (lp *Loadpoint) SetChargingRestrictions(s loadpoint.ChargingRestrictions) error {
	if err := s.Validate(); err != nil {
		return err
	}
	data, _ := json.Marshal(s)
	var copied loadpoint.ChargingRestrictions
	_ = json.Unmarshal(data, &copied)
	lp.Lock()
	if err := lp.settings.SetJson(keys.ChargingRestrictions, copied); err != nil {
		lp.Unlock()
		return err
	}
	lp.restrictions = copied
	lp.restrictionNext = time.Time{}
	lp.restrictionScheduled = false
	if lp.restrictionTimer != nil {
		lp.restrictionTimer.Stop()
		lp.restrictionTimer = nil
	}
	lp.Unlock()
	lp.publish(keys.ChargingRestrictions, s)
	lp.requestUpdate()
	return nil
}

func (lp *Loadpoint) restoreChargingRestrictions() {
	var s loadpoint.ChargingRestrictions
	if lp.settings.Json(keys.ChargingRestrictions, &s) == nil {
		if err := s.Validate(); err == nil {
			lp.restrictions = s
		} else {
			lp.log.ERROR.Println(err)
		}
	}
	lp.publish(keys.ChargingRestrictions, lp.GetChargingRestrictions())
}

func (lp *Loadpoint) scheduleBlocked() bool {
	return lp.GetMode() == api.ModeSmart && lp.GetChargingRestrictions().Blocked(lp.clock.Now())
}

func (lp *Loadpoint) publishChargingRestriction() {
	now := lp.clock.Now()
	lp.Lock()
	s := lp.restrictions
	if s.Enabled && (!lp.restrictionScheduled || !lp.restrictionNext.IsZero() && !now.Before(lp.restrictionNext)) {
		lp.restrictionNext = s.NextTransition(now)
		lp.restrictionScheduled = true
		if lp.restrictionTimer != nil {
			lp.restrictionTimer.Stop()
		}
		if !lp.restrictionNext.IsZero() {
			lp.restrictionTimer = lp.clock.AfterFunc(lp.restrictionNext.Sub(now), lp.requestUpdate)
		}
	}
	next := lp.restrictionNext
	lp.Unlock()
	lp.publish(keys.ScheduleBlocked, lp.GetMode() == api.ModeSmart && s.Blocked(now))
	var transition any
	if !next.IsZero() {
		transition = next
	}
	lp.publish(keys.ScheduleNextTransition, transition)
}

func (lp *Loadpoint) importAllowsEnable() bool {
	limiter, ok := lp.site.(interface {
		limitImportPower(*Loadpoint, float64) float64
	})
	if !ok {
		return true
	}
	minimum := grid.CurrentToPower(lp.effectiveMinCurrent(), lp.ActivePhases())
	return limiter.limitImportPower(lp, minimum) >= minimum
}
