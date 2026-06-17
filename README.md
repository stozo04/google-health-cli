# google-health-cli

A single, statically-compiled Go binary that reads your exercise sessions from the **Google Health API**
over read-only OAuth2, filters them to a configurable allowlist of exercise types, and upserts the matches
into a JSON daily-log file.

It is **self-contained**: it talks directly to the Google Health API over OAuth2. No Python interpreter and
no external helper binary are required.

## How it works

```
GET https://health.googleapis.com/v4/users/me/dataTypes/exercise/dataPoints   (read-only OAuth2)
        │   filter = exercise.interval.civil_start_time in [from, to)
        ▼
  parse each session  →  exercise_type, start, duration, avg HR, calories
        │
        ▼
  allowlist filter: keep exercise_type ∈ config.elliptical_types
  (every other session the API returns is dropped)
        │
        ▼
  merge by local date  →  upsert a training object into the daily-log JSON
```

The allowlist (`elliptical_types`) is the core filter: only the listed exercise types are kept; everything
else is ignored. Average heart rate is reported against a configurable Zone 2 band
(`zone2_low`–`zone2_high`) — the band is a readout only and never filters sessions.

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
google-health-cli sessions --days 14          # list all sessions; * marks an allowlisted (cardio) session
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

Each matched session is written into the day's `training` object in the daily-log JSON, tagged
`"type": "cardio"`:

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

`sync` targets a specific `DAILY_LOG.json` shape. The write is intentionally diff-stable — existing keys,
key order, the `"source": "ghealth"` marker, and the file's existing line endings are all preserved on write.
See **[AGENTS.md](AGENTS.md)** for the exact contract.

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
