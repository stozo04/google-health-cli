"""Read / merge / write DAILY_LOG.json.

Cardio is "just a different type of training" (Steven's call), so we write it
into the day's existing `training` object — same shape as a strength session
(`session` / `completed` / `source`) plus cardio fields (avg HR, Zone 2, etc.),
tagged `"type": "cardio"`.

Safety guard: we only write into `training` when the day has no training yet, or
when the existing entry was a prior ghealth write (so re-syncs stay idempotent).
A manually-logged session (strength OR cardio) is never clobbered — that day is
reported as a conflict and skipped for the human to resolve.
"""
import json
from datetime import date

WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]


def load(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save(path, doc):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(doc, f, ensure_ascii=False, indent=2)
        f.write("\n")


def zone2_label(avg_hr, low, high):
    """Calm, factual band readout — never a verdict, just where it landed."""
    if avg_hr is None:
        return "no HR"
    if avg_hr < low:
        return f"below band (<{low})"
    if avg_hr > high:
        return f"above band (>{high})"
    return f"in band ({low}-{high})"


def merge_sessions(sessions):
    """Combine 1+ elliptical sessions for one day into a single cardio summary.

    Duration and calories sum; avg HR is duration-weighted.
    """
    sessions = [s for s in sessions if s]
    if not sessions:
        return None
    total_min = sum(s.get("duration_min") or 0 for s in sessions)
    total_cal = sum(s.get("calories") or 0 for s in sessions)

    weighted, weight = 0.0, 0
    for s in sessions:
        if s.get("avg_hr") and s.get("duration_min"):
            weighted += s["avg_hr"] * s["duration_min"]
            weight += s["duration_min"]
    avg_hr = round(weighted / weight) if weight else (
        sessions[0].get("avg_hr") if len(sessions) == 1 else None)

    return {
        "duration_min": total_min or None,
        "calories": round(total_cal) if total_cal else None,
        "avg_hr": avg_hr,
        "count": len(sessions),
    }


def build_cardio_entry(sessions, cfg):
    """Build the `training` object for an elliptical cardio day."""
    summary = merge_sessions(sessions)
    if not summary:
        return None
    entry = {
        "session": "Zone 2 elliptical",
        "type": "cardio",
        "completed": True,
        "source": "ghealth",
        "duration_min": summary["duration_min"],
        "avg_hr": summary["avg_hr"],
        "calories": summary["calories"],
        "zone2": zone2_label(summary["avg_hr"], cfg["zone2_low"], cfg["zone2_high"]),
    }
    if summary["count"] > 1:
        entry["sessions"] = summary["count"]
    return entry


def find_day(doc, day_iso):
    for d in doc.get("days", []):
        if d.get("date") == day_iso:
            return d
    return None


def upsert_cardio(doc, day_iso, entry):
    """Write `entry` into day[day_iso].training, creating the day if needed.

    Returns (day_obj, status) where status is "created" | "updated" | "conflict".
    "conflict" means a non-ghealth training session is already logged that day;
    we leave it untouched so a manual entry is never destroyed.
    """
    day = find_day(doc, day_iso)
    if day is not None:
        existing = day.get("training")
        if existing and existing.get("source") != "ghealth":
            return day, "conflict"
        day["training"] = entry
        return day, ("updated" if existing else "created")

    wd = WEEKDAYS[date.fromisoformat(day_iso).weekday()]
    day = {
        "date": day_iso,
        "weekday": wd,
        "partial": True,
        "training": entry,
        "body": {"weight_lb": None, "waist_in": None},
        "watch": {"sleep_hrs": None, "resting_hr": None, "steps": None},
    }
    days = doc.setdefault("days", [])
    # Insert keeping the list sorted by date.
    for i, d in enumerate(days):
        if d.get("date", "") > day_iso:
            days.insert(i, day)
            break
    else:
        days.append(day)
    return day, "created"
