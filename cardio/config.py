"""Config loading: JSON file + environment-variable overrides.

This tool shells out to the `ghealth` binary (which owns all OAuth/token state),
so there are no credentials here. We only need to know:

  * ghealth_bin   - path to the ghealth executable
  * daily_log     - path to the Workout DAILY_LOG.json we append to
  * elliptical_types - the Google Health `exercise_type` values that count as
                    cardio for our purposes. ALLOWLIST: anything not in this set
                    (notably STRENGTH_TRAINING) is ignored, so we never double-
                    count the strength work speediance-cli already logs.
  * zone2_low/high - Steven's Zone 2 HR band (110-130 bpm) for the calm,
                    trend-based readout. Avg HR is the field that matters most.

A session's local date is taken from its own startTime + startUtcOffset (the
offset ships in the data), so no timezone config is needed.
"""
import os
import json

# Project root = the cardio-cli/ folder that contains this package.
_PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def _default_config_path():
    """config.json in CWD first (matches speediance-cli), else next to the
    package so `python -m cardio` works from any directory (e.g. the Monday
    check-in running from the Workout folder)."""
    if os.path.exists("config.json"):
        return "config.json"
    return os.path.join(_PROJECT_ROOT, "config.json")


CONFIG_PATH = os.environ.get("CARDIO_CONFIG", _default_config_path())

DEFAULTS = {
    # Path to the ghealth executable (the Google Health API client).
    "ghealth_bin": "ghealth",
    # The DAILY_LOG.json this tool appends cardio sessions to.
    "daily_log": "",
    # exercise_type values that count as elliptical/cross-trainer cardio.
    # Locked from Steven's real `data list exercise` output before first sync.
    # Kept as a list so it's easy to add ROWING / WALKING / RUNNING later.
    "elliptical_types": ["ELLIPTICAL"],
    # Steven's Zone 2 band (PROGRAM.md): 110-130 bpm.
    "zone2_low": 110,
    "zone2_high": 130,
}


def load_config():
    cfg = dict(DEFAULTS)
    if os.path.exists(CONFIG_PATH):
        with open(CONFIG_PATH, "r", encoding="utf-8") as f:
            cfg.update(json.load(f))
    cfg["ghealth_bin"] = os.environ.get("CARDIO_GHEALTH_BIN", cfg["ghealth_bin"])
    cfg["daily_log"] = os.environ.get("CARDIO_DAILY_LOG", cfg["daily_log"])
    if not cfg["daily_log"]:
        raise SystemExit(
            "Missing daily_log path. Set it in config.json or via CARDIO_DAILY_LOG. "
            "It should point at the Workout project's DAILY_LOG.json."
        )
    return cfg
