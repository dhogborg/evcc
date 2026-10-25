# Adaptive grid import control

In Configuration, General, select **Hourly import limit** below **Control behaviour**.
Enable it and enter the maximum imported energy in kWh per clock hour. The setting
is saved immediately without restarting. Zero disables the budget through the API.

The grid meter must expose cumulative imported energy. Exported energy never
replenishes the budget. All charging modes, including Fast, obey the import limit
and existing circuit limits. Other household loads are included in consumption but
cannot be switched off by this controller. This feature cannot guarantee a
whole-property limit when uncontrolled loads or charger failures exceed it.

Clock hours use Europe/Stockholm. The repeated hour when daylight-saving time ends
has two distinct budgets. Each control cycle computes:

```
remaining kWh = configured limit - imported kWh this hour
allowed grid kW = max(0, remaining kWh - reserve kWh) / hours until next full hour
```

The reserve is the larger of 0.05 kWh and two minutes of measured positive grid
import. Available charging power also accounts for the measured household load,
local generation, and battery flow. Outstanding charger offers reserve shared
headroom so several loadpoints cannot allocate the same power. If the allowance
is below a charger's minimum current, charging pauses. A fully spent budget may
still allow charging from sufficient local generation without grid import.

Hour accounting continues while the limit is disabled. The cumulative meter
checkpoint is persisted using the existing settings store. A restart within an
accounted hour includes import during downtime. A first start mid-hour or an
unknown boundary pauses controlled charging until the next measurable hour.
A sample spanning an hour boundary is conservatively counted entirely against
the new hour; the preceding hour also retains the actuation reserve. Missing or
non-finite meter readings pause charging. A decreasing register or changed meter
invalidates that hour. Data older than two minutes is considered stale.

The modal shows imported and remaining energy, calculated grid power allowance,
and why charging is paused. A limit reduction never resets energy already used.

## Loadpoint restrictions

Open a loadpoint's settings and find **Charging restrictions** at the bottom.
Schedules are independent per loadpoint and apply only in Smart mode. Fast
bypasses the schedule but continues to obey the hourly budget.

Each rule has months, ISO weekdays (Monday=1, Sunday=7), start and end times, and
uses the schedule's IANA timezone. Empty month or weekday selections match all.
Starts are inclusive and ends exclusive. For example, weekdays in January,
February, March, November and December with `07:00` to `20:00` block Smart
charging through 19:59:59. Public holidays are treated as ordinary weekdays.

An end before the start spans midnight; matching months and weekdays refer to
the starting date. Equal start and end times are rejected. Overlapping rules
combine as a union. Repeated local times are evaluated twice; nonexistent local
times are skipped. A timer requests a control update at each transition, subject
to meter and charger response latency. The selected charging mode and plans are
preserved while blocked. Welcome charging, minimum SoC, and optimizer suggestions
cannot bypass an active Smart restriction.

## API

- `POST /api/hourlyimportlimit/14` sets 14 kWh per clock hour.
- `POST /api/hourlyimportlimit/0` disables the budget.
- `GET /api/loadpoints/1/chargingrestrictions` returns the saved schedule.
- `POST /api/loadpoints/1/chargingrestrictions` replaces it with:

```json
{
  "enabled": true,
  "timezone": "Europe/Stockholm",
  "rules": [
    {
      "months": [1, 2, 3, 11, 12],
      "weekdays": [1, 2, 3, 4, 5],
      "start": "07:00",
      "end": "20:00"
    }
  ]
}
```

`/api/state` exposes `hourlyImportLimit` and `hourlyImport` (`used`, `remaining`,
`allowedPower` in W, `reset`, `status`). Each loadpoint exposes
`chargingRestrictions`, `scheduleBlocked`, and `scheduleNextTransition`.
