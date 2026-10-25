package core

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/evcc-io/evcc/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportAccountingRecovery(t *testing.T) {
	at := time.Date(2026, 11, 2, 12, 59, 30, 0, time.UTC)
	var c importCheckpoint
	c.observe(at, "grid", 100)
	assert.False(t, c.Valid, "a mid-hour start cannot assume unused budget")
	c.observe(at.Add(time.Minute), "grid", 100.2)
	assert.True(t, c.Valid)
	assert.InDelta(t, .2, c.Used, 1e-9, "count the entire boundary-spanning delta")
	data, err := json.Marshal(c)
	require.NoError(t, err)
	var restored importCheckpoint
	require.NoError(t, json.Unmarshal(data, &restored))
	restored.observe(at.Add(10*time.Minute), "grid", 102.2)
	assert.True(t, restored.Valid)
	assert.InDelta(t, 2.2, restored.Used, 1e-9, "include import during downtime")
	restored.observe(at.Add(11*time.Minute), "grid", 1)
	assert.False(t, restored.Valid, "a meter reset invalidates the hour")
	restored.observe(at.Add(12*time.Minute), "grid", 2)
	assert.False(t, restored.Valid, "later readings cannot invent the lost baseline")
}

func TestImportUnknownBoundaryAndMeterReplacement(t *testing.T) {
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	c := importCheckpoint{Meter: "grid", Hour: at, At: at, Energy: 100, Valid: true}
	c.observe(at.Add(90*time.Minute), "grid", 110)
	assert.False(t, c.Valid, "cannot reconstruct the next hour after a long gap across its boundary")
	c.observe(at.Add(91*time.Minute), "replacement", 111)
	assert.False(t, c.Valid)
}

func TestImportBudgetPowerAndAggregateReservation(t *testing.T) {
	now := time.Now()
	c := hourlyImportControl{limit: 14, fresh: true, gridPower: 3000, checkpoint: importCheckpoint{Hour: importHour(now), At: now, Used: 8, Valid: true}}
	status := c.status(now)
	assert.Equal(t, "active", status.Status)
	assert.InDelta(t, (6-.1)/status.Reset.Sub(now).Hours()*1000, status.AllowedPower, 1e-6)
	c.limit = 7
	assert.Equal(t, "exhausted", c.status(now).Status)
	assert.Zero(t, c.status(now).AllowedPower)
	c.fresh = false
	assert.Equal(t, "meterUnavailable", c.status(now).Status)

	site := new(Site)
	site.importControl.limit = 14
	site.importControl.fresh = true
	site.importControl.checkpoint = importCheckpoint{Hour: importHour(now), At: now, Used: 8, Valid: true}
	a, b := new(Loadpoint), new(Loadpoint)
	cap := site.importControl.status(time.Now()).AllowedPower
	grantedA := site.limitImportPower(a, cap*.75)
	grantedB := site.limitImportPower(b, cap*.75)
	assert.InDelta(t, cap, grantedA+grantedB, 1, "loadpoints share one aggregate allowance")
	site.importControl.fresh = false
	assert.Zero(t, site.limitImportPower(a, 5000), "stale meters fail closed in every mode")
}

func TestImportHourDST(t *testing.T) {
	a, _ := time.Parse(time.RFC3339, "2026-10-25T02:30:00+02:00")
	b, _ := time.Parse(time.RFC3339, "2026-10-25T02:30:00+01:00")
	assert.Equal(t, time.Hour, importHour(b).Sub(importHour(a)))
}

func TestImportReadingsFailClosed(t *testing.T) {
	site := new(Site)
	energy := 100.0
	site.recordHourlyImport(time.Now(), types.Measurement{Name: "grid", Energy: &energy, Power: 2000})
	assert.True(t, site.importControl.fresh)
	site.recordHourlyImport(time.Now(), types.Measurement{Name: "grid", Power: 2000})
	assert.False(t, site.importControl.fresh)
	energy = math.NaN()
	site.recordHourlyImport(time.Now(), types.Measurement{Name: "grid", Energy: &energy})
	assert.False(t, site.importControl.fresh)
}
