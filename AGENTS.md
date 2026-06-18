# AGENTS.md — machine contract for google-health-cli

google-health-cli is a read-only Google Health data extractor. It emits your health data as JSON; it does
no filtering, merging, or interpretation. **stdout is data; stderr is hints/logs/errors.** Pass `--json` for
machine-readable output (some commands are JSON-only — noted below).

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success |
| `2` | Auth / API failure (no/invalid token, refresh failure, non-2xx response, bad JSON) — and `doctor` when not authenticated |
| `64` | Usage error (bad flags, bad `--date`, unknown data type, non-listable type, or an expired token with no OAuth client credentials to refresh it) |
| `78` | Config error (unreadable / invalid `config.json`; `doctor` when no config / no `client_id` is found) |
| `1` | Other failure |

When not authenticated, API commands print `Not authenticated. Run:  google-health-cli auth login` to stderr
and exit `2`.

When the cached token has **expired** and no OAuth client credentials were resolved (no config found, or a
config without `client_id`/`client_secret`), API commands fail fast with exit `64` and an actionable message
naming the discovery order — rather than the cryptic `oauth2 "Could not determine client ID"` error from the
token endpoint. Run `doctor` to diagnose it.

## `data list <type>` (the core)

Lists data points for one data type over a time window. **stdout is always a JSON array of the raw API data
points** (verbatim, never reshaped); a one-line `N <type> data point(s)` count goes to stderr. `--json` is
accepted but is a no-op (output is always JSON).

```sh
google-health-cli data list heart-rate --days 1
```

```json
[
  { "dataSource": { "...": "..." }, "heartRate": { "...": "..." } },
  { "...": "..." }
]
```

Window flags (precedence: `--all` > `--from`/`--to` > `--date`/`--days`):

| Flag | Meaning | Default |
|---|---|---|
| `--date` | civil anchor: `today` \| `yesterday` \| `YYYY-MM-DD` | `today` |
| `--days N` | days back from `--date` | `7` |
| `--from` / `--to` | explicit window, RFC3339 (must be given together) | — |
| `--all` | ignore the window; list everything for the type | off |

The time filter is built on the type's default time field, formatted per record family (civil wall-clock,
RFC3339 instant, or date-only). Unknown type → exit `64`. A rollup/reconcile-only type (no `list`
operation) → exit `64` with a message. If the API rejects the filter, re-run with `--all`.

## `rollup daily <type>` (server-side daily totals)

Returns the API's **daily rollup** rows — one reconciled value per civil (local) calendar day — instead of
the raw per-minute/per-sample points `data list` returns. Use it to get daily totals cheaply (e.g. a
`steps` total per day rather than ~1 MB/week of per-minute points), and as the **only** way to read the
rollup-only types that have no `list` operation (`active-minutes`, `total-calories`, `floors`,
`calories-in-heart-rate-zone`, `time-in-heart-rate-zone`). Read-only: `dailyRollUp` is an HTTP POST but a
pure query, no write scopes.

**stdout is always a JSON array of the raw rollup rows** (verbatim, never reshaped); a one-line
`N <type> daily rollup(s)` count goes to stderr. `--json` is accepted but a no-op.

```sh
google-health-cli rollup daily steps --days 7
```

```json
[
  {
    "civilStartTime": { "date": { "year": 2026, "month": 6, "day": 16 }, "time": {} },
    "civilEndTime":   { "date": { "year": 2026, "month": 6, "day": 17 }, "time": {} },
    "steps": { "countSum": "8000" }
  }
]
```

Each row carries `civilStartTime`/`civilEndTime` (the `[day, day+1)` civil window it aggregates) and one
type-specific value (`steps.countSum`, `totalCalories.kcalSum`, `distance.millimetersSum`,
`activeMinutes.activeMinutesRollupByActivityLevel[]`, …). Bucketing is on the **civil/local calendar day**
(the request uses zone-free civil dates), and each value is **reconciled across all data sources and
excludes off-wrist intervals** — so it does *not* equal a naive sum of `data list` points (those are
unreconciled and double-count overlapping sources). Prefer this over re-summing raw points.

Window flags mirror `data list` (`--date`/`--days` default, or explicit `--from`/`--to`), minus `--all`:
`dailyRollUp` requires a bounded range. The API caps the range per type (e.g. **90 days for `steps`**); an
over-long window → exit `2` with the API's `…must not exceed N days` detail. Unknown type → exit `64`. A
type that does not support `dailyRollUp` → exit `64` with a message pointing to `data list`.

## `types list` / `types describe <type>`

Discovery, no network. `types list --json` is an array; `types describe <type> --json` is one object. Key
order frozen:

```json
{
  "name": "Heart Rate",
  "endpoint_name": "heart-rate",
  "filter_name": "heart_rate",
  "record_type": "Sample",
  "operations": ["list", "reconcile", "rollup", "dailyRollUp"],
  "listable": true,
  "scope": "health_metrics_and_measurements",
  "read_scope": "https://www.googleapis.com/auth/googlehealth.health_metrics_and_measurements.readonly",
  "default_time_path": "heart_rate.sample_time.physical_time"
}
```

`types list` (human) marks listable types with `*`. There are 31 types.

## `sessions` (parsed exercise convenience)

A flattened, parsed view of the `exercise` type — sugar over `data list exercise`. No filtering: every
exercise type is returned. `sessions --json` is an array (empty → `[]`), sorted by start ascending, field
order frozen:

```json
[
  {
    "date": "2026-06-16",
    "exercise_type": "ELLIPTICAL",
    "duration_min": 35,
    "avg_hr": 130,
    "calories": 236,
    "platform": "FITBIT"
  }
]
```

`sessions --raw` dumps the raw exercise data points (indent-2 JSON).

## `api get <path>` (read-only escape hatch)

Authenticated GET to any v4 path; prints the response (re-indented if JSON). For endpoints the typed
surface doesn't model — `users/me/profile`, `users/me/settings`, a single dataPoint by name. Only GET is
offered. Exit `2` on non-2xx.

```sh
google-health-cli api get /v4/users/me/profile
```

## `doctor` (always JSON on stdout)

```json
{
  "tokenValid": true,
  "baseURL": "https://health.googleapis.com",
  "user": "users/me",
  "tokenPath": "<token cache path>",
  "configPath": "<config.json path>",
  "configFound": true,
  "clientIdLoaded": true,
  "scopes": ["...readonly", "..."],
  "version": "<version>"
}
```

`tokenValid` is the frozen key. `configFound` is whether `configPath` exists on disk; `clientIdLoaded`
is whether an OAuth `client_id` was resolved (from the file or env). The JSON is always printed first, then:

- exit `78` (with a stderr warning naming the search order) when no config was found or it lacks
  `client_id`/`client_secret` — this is the root cause of the cryptic `oauth2 "Could not determine client
  ID"` refresh failure, so `doctor` flags it explicitly rather than staying silent;
- otherwise exit `2` when `tokenValid` is `false`.

## `auth status --json`

```json
{ "present": true, "valid": true, "expiry": "2026-06-17T18:00:00Z", "token_path": "<path>" }
```

`valid` is a local, non-expired check (no network call).

## `config show --json`

The resolved effective config; `client_secret` is always blanked. Key order frozen:

```json
{
  "client_id": "...",
  "client_secret": "",
  "base_url": "https://health.googleapis.com",
  "user": "users/me",
  "token_cache": "<path>",
  "scopes": ["...", "..."],
  "config_path": "<path>"
}
```
