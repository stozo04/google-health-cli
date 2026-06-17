# AGENTS.md — machine contract for google-health-cli

This file documents the stable surfaces that automation and agents depend on. **stdout is
data; stderr is hints/logs/errors.** Pass `--json` for machine-readable output.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `2` | Auth / API failure (no/invalid token, refresh failure, non-2xx, bad JSON) — and `doctor` when not authenticated |
| `64` | Usage error (bad flags / bad `--date`) |
| `78` | Config error (missing `daily_log`, unreadable/invalid `config.json`) |
| `1` | Other failure |

When not authenticated, API commands print `Not authenticated. Run:  google-health-cli auth login` to stderr
and exit `2`.

## `doctor` (always JSON on stdout)

```json
{
  "tokenValid": true,
  "baseURL": "https://health.googleapis.com",
  "user": "users/me",
  "tokenPath": "<token cache path>",
  "configPath": "<config.json path>",
  "dailyLog": "<daily_log path>",
  "scopes": ["https://www.googleapis.com/auth/googlehealth.activity_and_fitness.readonly"],
  "version": "<version>"
}
```

`tokenValid` is the frozen key carried over from the previous tool. Exit `2` when `tokenValid` is `false`
(the JSON is still printed first).

## `sessions --json`

An array (empty → `[]`, never `null`), sorted by start ascending. Field order is frozen:

```json
[
  {
    "date": "2026-06-16",
    "exercise_type": "ELLIPTICAL",
    "elliptical": true,
    "duration_min": 35,
    "avg_hr": 130,
    "calories": 236,
    "platform": "FITBIT"
  }
]
```

`sessions --raw` dumps the raw Google Health data points (indent-2 JSON) for debugging.

## `sync --json`

```json
{
  "target": "2026-06-16",
  "days": 3,
  "dry_run": false,
  "dropped_non_cardio": 5,
  "results": [
    {
      "date": "2026-06-16",
      "duration_min": 35,
      "avg_hr": 130,
      "calories": 236,
      "zone2": "in band (110-130)",
      "status": "created"
    }
  ]
}
```

`results` is empty (`[]`) when nothing matched. `status` ∈ `created | updated | conflict | dry-run`:

- `created` — a new day, or training added to a day that lacked it.
- `updated` — a prior `ghealth` entry overwritten in place (idempotent re-sync).
- `conflict` — a manually-logged training session is present; left untouched.
- `dry-run` — `--dry-run` was set; nothing was written.

When `--json` is set, stdout is JSON only; human hints go to stderr.

## DAILY_LOG.json write

The `training` object written for an elliptical day (key order frozen):

```json
{
  "session": "Elliptical",
  "type": "cardio",
  "completed": true,
  "source": "ghealth",
  "duration_min": 35,
  "avg_hr": 130,
  "calories": 236,
  "zone2": "in band (110-130)"
}
```

`"sessions": <n>` is appended only when more than one session merged that day. The `"source": "ghealth"`
marker is load-bearing for idempotency and must not change. Existing keys, key order, and the file's line
endings are preserved on write.

## `auth status --json`

```json
{ "present": true, "valid": true, "expiry": "2026-06-17T18:00:00Z", "token_path": "<path>" }
```

`valid` is a local, non-expired check (no network call). `config show --json` redacts `client_secret`.
