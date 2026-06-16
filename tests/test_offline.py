"""Offline logic tests — no network, no ghealth, no auth required.

Exercises the parse -> filter -> merge -> upsert path with synthetic payloads
shaped like plausible Google Health Exercise sessions. The exact real field
names get confirmed against live data, but this proves the wiring/guards now.
"""
import copy
from google_health import health, daily_log as logmod

CFG = {
    "elliptical_types": ["ELLIPTICAL"],
    "zone2_low": 110, "zone2_high": 130, "tz": "America/Chicago",
}

# Synthetic data points shaped like the REAL Google Health response
# (confirmed from Steven's Pixel Watch). UTC times + a -18000s (CST) offset that
# pulls the local date back into 2026-06-16.
def _point(etype, start_utc, end_utc, hr, kcal, dur, name="Activity"):
    return {
        "exercise": {
            "exerciseType": etype,
            "displayName": name,
            "activeDuration": dur,
            "interval": {
                "startTime": start_utc, "startUtcOffset": "-18000s",
                "endTime": end_utc, "endUtcOffset": "-18000s",
            },
            "metricsSummary": {
                "averageHeartRateBeatsPerMinute": hr, "caloriesKcal": kcal,
            },
            "dataSource": {"platform": "FITBIT"},
        },
        "name": f"users/me/dataTypes/exercise/dataPoints/{name}",
    }


# 23:00Z - 18000s = 18:00 local on 2026-06-16.
ELLIPTICAL = _point("ELLIPTICAL", "2026-06-16T23:00:00Z", "2026-06-16T23:30:00Z",
                    "122", 245, "1800s", name="Elliptical")
STRENGTH = _point("STRENGTH_TRAINING", "2026-06-16T17:00:00Z", "2026-06-16T17:45:00Z",
                  "110", 200, "2700s", name="Strength training")


def test_parse_pulls_fields():
    s = health.parse_session(ELLIPTICAL)
    assert s["exercise_type"] == "ELLIPTICAL"
    assert s["duration_min"] == 30
    assert s["avg_hr"] == 122 and s["calories"] == 245
    assert s["start"].date().isoformat() == "2026-06-16"
    assert s["start"].hour == 18  # UTC 23:00 + (-5h) offset = 18:00 local


def test_filter_is_allowlist():
    assert health.is_elliptical(health.parse_session(ELLIPTICAL), CFG) is True
    assert health.is_elliptical(health.parse_session(STRENGTH), CFG) is False
    # Unknown / missing type must NOT pass (no double-count).
    assert health.is_elliptical({"exercise_type": None}, CFG) is False
    assert health.is_elliptical({"exercise_type": "WALKING"}, CFG) is False


def test_zone2_label():
    assert "in band" in logmod.zone2_label(122, 110, 130)
    assert "below" in logmod.zone2_label(95, 110, 130)
    assert "above" in logmod.zone2_label(150, 110, 130)


def test_merge_multiple_sessions():
    a = health.parse_session(ELLIPTICAL)
    b = health.parse_session(ELLIPTICAL)
    m = logmod.merge_sessions([a, b])
    assert m["duration_min"] == 60 and m["calories"] == 490 and m["count"] == 2
    assert m["avg_hr"] == 122  # duration-weighted, same HR


def test_upsert_creates_then_updates_idempotently():
    doc = {"days": [{"date": "2026-06-16", "weekday": "Tue",
                     "nutrition": {"calories_food": 1500}}]}
    entry = logmod.build_cardio_entry([health.parse_session(ELLIPTICAL)], CFG)
    _, status = logmod.upsert_cardio(doc, "2026-06-16", entry)
    assert status == "created"
    day = logmod.find_day(doc, "2026-06-16")
    assert day["training"]["type"] == "cardio" and day["training"]["avg_hr"] == 122
    assert day["nutrition"]["calories_food"] == 1500  # untouched
    # Re-sync overwrites our own entry, not a duplicate.
    _, status2 = logmod.upsert_cardio(doc, "2026-06-16", entry)
    assert status2 == "updated"


def test_upsert_never_clobbers_manual():
    doc = {"days": [{"date": "2026-06-16", "weekday": "Tue",
                     "training": {"session": "Push", "source": "manual"}}]}
    entry = logmod.build_cardio_entry([health.parse_session(ELLIPTICAL)], CFG)
    _, status = logmod.upsert_cardio(doc, "2026-06-16", entry)
    assert status == "conflict"
    assert logmod.find_day(doc, "2026-06-16")["training"]["session"] == "Push"


def test_upsert_creates_new_day_in_order():
    doc = {"days": [{"date": "2026-06-14"}, {"date": "2026-06-18"}]}
    entry = logmod.build_cardio_entry([health.parse_session(ELLIPTICAL)], CFG)
    logmod.upsert_cardio(doc, "2026-06-16", entry)
    assert [d["date"] for d in doc["days"]] == ["2026-06-14", "2026-06-16", "2026-06-18"]


if __name__ == "__main__":
    fns = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    for fn in fns:
        fn()
        print(f"  ok  {fn.__name__}")
    print(f"\n{len(fns)} passed.")
