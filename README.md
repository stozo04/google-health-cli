# google-health-cli

The **cardio** counterpart to [`speediance-cli`](https://github.com/stozo04/speediance-cli).

- `speediance-cli` pulls Steven's **strength** sessions from the Speediance cloud → writes weights/reps into `WEEKS/Week-XX.md`.
- `google-health-cli` pulls Steven's **elliptical / Zone 2** sessions from the **Google Health API** (via the [`ghealth`](https://github.com/rudrankriyam/Google-Health-CLI) binary) → writes them into `DAILY_LOG.json`.

His Google Watch logs **both** the elliptical and the Speediance strength workouts into Google Health. So the core job of this tool is the **dedup filter**: it keeps **only** elliptical/cross-trainer sessions and drops strength training, otherwise we'd double-count the strength work `speediance-cli` already owns.

## How it works

```
ghealth data list exercise --from … --to …   (Google Health API, read-only)
        │
        ▼
  parse each session  →  exercise_type, start, duration, avg HR, calories
        │
        ▼
  ALLOWLIST filter: keep exercise_type ∈ config.elliptical_types
  (everything else — STRENGTH_TRAINING, etc. — is ignored)
        │
        ▼
  merge by local date  →  upsert a `cardio` object into DAILY_LOG.json
```

Avg heart rate is the field that matters most — Steven's Zone 2 target is **110–130 bpm**, and each entry gets a calm `zone2` band label.

## Setup

1. Install `ghealth` and authenticate it (see `OAUTH_SETUP.md`). `cardio doctor` checks this.
2. `copy config.example.json config.json` and confirm the paths.
3. Lock the `elliptical_types` allowlist from real data:
   ```sh
   python -m google_health sessions --days 14          # see how each session is tagged
   ```
   Put the elliptical `exercise_type` value(s) into `config.json`.

## Usage

```sh
python -m google_health doctor                 # is ghealth installed + logged in?
python -m google_health sessions --days 14     # list all sessions, ✓ marks cardio
python -m google_health sessions --raw         # dump raw API JSON (for debugging)
python -m google_health sync --dry-run         # preview what would be written
python -m google_health sync --date yesterday  # write yesterday's cardio
python -m google_health sync --days 7          # backfill the last 7 days
```

`sync` is idempotent per day — re-running overwrites that day's `cardio` rather than duplicating.

## What it writes

Cardio is "just a different type of training", so it goes into the day's
`training` object (same shape as a strength session, tagged `"type": "cardio"`):

```json
"training": {
  "session": "Zone 2 elliptical",
  "type": "cardio",
  "completed": true,
  "source": "ghealth",
  "duration_min": 30,
  "avg_hr": 122,
  "max_hr": 138,
  "calories": 245,
  "zone2": "in band (110-130)"
}
```

A day whose `training` was logged manually (any non-ghealth source) is reported
as a **conflict and skipped**, never overwritten.

## Notes

- Read-only Google Health scopes. `ghealth` stores its OAuth token at
  `%AppData%\ghealth\token.json` (plaintext, user-only perms) — don't commit it.
- All HTTP/auth is owned by `ghealth`; this package is pure-stdlib Python.
