"""Talk to the Google Health API by shelling out to the `ghealth` binary.

ghealth owns OAuth/token state and the HTTP surface; we just call it and parse
JSON. Field paths below are confirmed against Steven's real `data list exercise`
output (Pixel Watch / Fitbit). A real session looks like:

  { "exercise": {
      "exerciseType": "ELLIPTICAL",
      "activeDuration": "1270s",
      "displayName": "Elliptical",
      "interval": { "startTime": "2026-06-02T20:45:00Z", "startUtcOffset": "-18000s",
                    "endTime":   "2026-06-02T21:06:10Z", "endUtcOffset":   "-18000s" },
      "metricsSummary": { "averageHeartRateBeatsPerMinute": "145", "caloriesKcal": 189 }
    },
    "dataSource": { "platform": "FITBIT", ... },
    "name": "users/.../dataPoints/8794835477193874423" }
"""
import json
import subprocess
from datetime import datetime, timedelta


class GHealthError(RuntimeError):
    pass


def run_ghealth(cfg, args, expect_json=True):
    """Run `ghealth <args> --json` and return parsed JSON (or raw text)."""
    cmd = [cfg["ghealth_bin"], *args]
    if expect_json and "--json" not in args and "--format" not in args:
        cmd.append("--json")
    proc = subprocess.run(cmd, capture_output=True, text=True)
    out = (proc.stdout or "").strip()
    err = (proc.stderr or "").strip()
    if proc.returncode != 0:
        msg = err or out or f"ghealth exited {proc.returncode}"
        try:
            j = json.loads(err or out)
            msg = j.get("message", msg)
            if j.get("hint"):
                msg += f"  (hint: {j['hint']})"
        except Exception:
            pass
        raise GHealthError(msg)
    if not expect_json:
        return out
    if not out:
        return None
    try:
        return json.loads(out)
    except json.JSONDecodeError as e:
        raise GHealthError(f"ghealth returned non-JSON output: {e}\n{out[:500]}")


def _civil(dt):
    """Civil (local wall-clock) datetime string, NO trailing Z.

    The exercise endpoint filters on `exercise.interval.civil_start_time`, a
    civil date-time; a UTC 'Z' is rejected with
    INVALID_DATA_POINT_FILTER_CIVIL_DATE_TIME_FORMAT.
    """
    return dt.strftime("%Y-%m-%dT%H:%M:%S")


def list_exercise_sessions(cfg, start_local, end_local):
    """Return the raw exercise data points whose civil start is in the window.

    `start_local` / `end_local` are naive local (civil) datetimes.
    """
    data = run_ghealth(cfg, [
        "data", "list", "exercise",
        "--from", _civil(start_local),
        "--to", _civil(end_local),
    ])
    return _as_point_list(data)


def _as_point_list(data):
    """Normalize ghealth's list response to a flat list of data-point objects."""
    if data is None:
        return []
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        for key in ("dataPoints", "data_points", "points", "items"):
            if isinstance(data.get(key), list):
                return data[key]
        return [data]
    return []


# ---- Field extraction (confirmed against real data) -------------------------

def _parse_utc(s):
    if not s:
        return None
    try:
        return datetime.fromisoformat(str(s).replace("Z", "+00:00"))
    except ValueError:
        return None


def _offset_seconds(s):
    """'-18000s' -> -18000."""
    if s is None:
        return 0
    try:
        return int(str(s).rstrip("s"))
    except ValueError:
        return 0


def _local_dt(interval, which):
    """Local wall-clock datetime (naive) for interval start/end.

    local = UTC instant + its UTC offset. Using the per-session offset means
    dates bucket correctly regardless of DST or where the data was recorded.
    """
    utc = _parse_utc(interval.get(which + "Time"))
    if utc is None:
        return None
    local = utc + timedelta(seconds=_offset_seconds(interval.get(which + "UtcOffset")))
    return local.replace(tzinfo=None)


def _duration_secs(s):
    """'1270s' -> 1270."""
    if not s:
        return None
    try:
        return int(float(str(s).rstrip("s")))
    except ValueError:
        return None


def _to_num(v):
    if isinstance(v, bool):
        return None
    if isinstance(v, (int, float)):
        return v
    if isinstance(v, str):
        try:
            return float(v) if "." in v else int(v)
        except ValueError:
            return None
    return None


def parse_session(point):
    """Normalize one raw exercise data point into the fields we log.

    Never raises — a malformed session yields Nones rather than breaking a sync.
    """
    ex = point.get("exercise") if isinstance(point.get("exercise"), dict) else point
    interval = ex.get("interval") or {}
    start = _local_dt(interval, "start")
    end = _local_dt(interval, "end")

    secs = _duration_secs(ex.get("activeDuration"))
    if secs is None and start and end and end > start:
        secs = (end - start).total_seconds()
    duration_min = round(secs / 60) if secs else None

    summary = ex.get("metricsSummary") or {}
    src = ex.get("dataSource") or point.get("dataSource") or {}

    return {
        "exercise_type": ex.get("exerciseType"),
        "display_name": ex.get("displayName"),
        "start": start,
        "end": end,
        "duration_min": duration_min,
        "avg_hr": _to_num(summary.get("averageHeartRateBeatsPerMinute")),
        "calories": _to_num(summary.get("caloriesKcal")),
        "platform": src.get("platform"),
        "point_id": point.get("name"),
    }


def is_elliptical(session, cfg):
    """ALLOWLIST filter — the dedup guard. True only for configured cardio types.

    Anything else (STRENGTH_TRAINING, CARDIO_WORKOUT, unknown, None) returns
    False and is ignored, so strength sessions speediance-cli already logs are
    never double-counted here.
    """
    etype = session.get("exercise_type")
    if not etype:
        return False
    allowed = {t.upper() for t in cfg.get("elliptical_types", [])}
    return str(etype).upper() in allowed
