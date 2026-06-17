# google-health-cli

The **cardio** counterpart to [`speediance-cli`](https://github.com/stozo04/speediance-cli) — a single,
statically-compiled Go binary that pulls your **elliptical / cross-trainer** sessions from the
**Google Health API** and writes them into `personal-workout-ai`'s `DAILY_LOG.json`.

- `speediance-cli` pulls **strength** sessions from the Speediance cloud → writes weights/reps into `WEEKS/Week-XX.md`.
- `google-health-cli` pulls **elliptical** sessions from the **Google Health API** → upserts them into `DAILY_LOG.json`.

Your Pixel Watch logs **both** the elliptical and the Speediance strength workouts into Google Health, so the
core job of this tool is the **dedup filter**: keep **only** elliptical/cross-trainer sessions and drop strength
training, otherwise it would double-count the strength work `speediance-cli` already owns.

It is **self-contained**: it talks directly to the Google Health API over OAuth2. No Python interpreter and no
`ghealth` binary are required.

## How it works

```
GET https://health.googleapis.com/v4/users/me/dataTypes/exercise/dataPoints   (read-only OAuth2)
        │   filter = exercise.interval.civil_start_time in [from, to)
        ▼
  parse each session  →  exercise_type, start, duration, avg HR, calories
        │
        ▼
  ALLOWLIST filter: keep exercise_type ∈ config.elliptical_types
  (everything else — STRENGTH_TRAINING, CARDIO_WORKOUT, … — is ignored)
        │
        ▼
  merge by local date  →  upsert a `cardio` training object into DAILY_LOG.json
```

Avg heart rate is the field that matters most. Steven's Zone 2 target is **110–130 bpm**, and **every**
elliptical session is logged regardless of heart rate — the band is a calm `zone2` readout, never a filter.

## Install

Download a binary from the [releases page](https://github.com/stozo04/google-health-cli/releases), or:

```sh
go install github.com/stozo04/google-health-cli/cmd/google-health-cli@latest
```

## Setup

1. Create a Google Cloud OAuth client and authorize the tool (one time, ~10 min). See **[OAUTH_SETUP.md](OAUTH_SETUP.md)**.
2. `copy config.example.json config.json` and fill in `daily_log`, `client_id`, `client_secret`.
3. Authorize and confirm:
   ```sh
   google-health-cli auth login     # opens a browser, read-only scopes, caches a token
   google-health-cli doctor         # expect  "tokenValid": true
   ```

## Usage

```sh
google-health-cli doctor                      # config + token check (JSON; exit 2 if not authenticated)
google-health-cli sessions --days 14          # list all sessions; * marks cardio
google-health-cli sessions --raw              # dump raw API JSON (for debugging)
google-health-cli sessions --json             # machine-readable rows
google-health-cli sync --dry-run              # preview what would be written
google-health-cli sync --date yesterday       # write yesterday's cardio
google-health-cli sync --days 7               # backfill the last 7 days
google-health-cli sync --json                 # machine-readable sync summary

google-health-cli auth login | logout | status
google-health-cli config show [--json] | config path
google-health-cli version [--json]
google-health-cli completion bash|zsh|fish|powershell
```

`sync` is idempotent per day — re-running overwrites that day's cardio entry rather than duplicating, and a
day whose `training` was logged manually (any non-`ghealth` source) is reported as a **conflict and skipped**,
never overwritten.

`--date` accepts `today` | `yesterday` | `YYYY-MM-DD`. stdout is data (parseable with `--json`); human hints
and logs go to stderr.

## What it writes

Cardio is "just a different type of training", so it goes into the day's `training` object (same shape as a
strength session, tagged `"type": "cardio"`):

```json
"training": {
  "session": "Elliptical",
  "type": "cardio",
  "completed": true,
  "source": "ghealth",
  "duration_min": 30,
  "avg_hr": 122,
  "calories": 245,
  "zone2": "in band (110-130)"
}
```

The `DAILY_LOG.json` write is byte-for-byte compatible with the previous Python tool (key order preserved, the
`"source": "ghealth"` marker kept, existing line endings preserved), so the diff stays clean.

## Configuration

`config.json` is discovered via `--config` → `$GOOGLE_HEALTH_CONFIG` → `./config.json` → next to the
executable. Precedence is flags > env > file > defaults.

| Key | Default | Env override |
|---|---|---|
| `daily_log` | *(required)* | `GOOGLE_HEALTH_DAILY_LOG` |
| `elliptical_types` | `["ELLIPTICAL"]` | — |
| `zone2_low` / `zone2_high` | `110` / `130` | — |
| `client_id` | `""` | `GOOGLE_HEALTH_CLIENT_ID` |
| `client_secret` | `""` | `GOOGLE_HEALTH_CLIENT_SECRET` |
| `base_url` | `https://health.googleapis.com` | `GOOGLE_HEALTH_BASE_URL` |
| `user` | `users/me` | — |
| `token_cache` | `<user config dir>/google-health-cli/token.json` | `GOOGLE_HEALTH_TOKEN_CACHE` |
| `scopes` | `[".../googlehealth.activity_and_fitness.readonly"]` | — |

`config.json` and the token cache hold credentials — they are written `0600` and must never be committed.

## Notes

- Read-only Google Health scope only (`googlehealth.activity_and_fitness.readonly`). The tool never requests
  write scopes and makes no network calls other than Google OAuth + the Health API.
- The OAuth token is cached locally (`0600`) and auto-refreshed; the refreshed token is re-persisted so it
  stays valid between runs.
- For agent/automation consumers, see **[AGENTS.md](AGENTS.md)** for the `--json` shapes and exit codes.
