"""Generate byte-exact golden files from the REAL Python google-health-cli.

These goldens are the parity oracle for the Go port (GOAL.md §14). Do NOT
hand-author the golden files — regenerate them with this harness:

    python tools/gen_golden.py

It drives the actual google_health package (imported from the sibling Python
repo) with the network stubbed out, so the goldens reflect exactly what the
Python tool emits for the committed fixture.

The one intentional behavioral fix from GOAL.md §11 is applied here before
generating the DAILY_LOG goldens: build_cardio_entry's session name is the
zone-neutral "Elliptical" instead of the Python's hardcoded "Zone 2 elliptical".
Both sides agree on this single divergence; the Python source is removed at
cutover anyway.
"""

import contextlib
import io
import json
import pathlib
import shutil
import sys
import tempfile

PY_REPO = r"C:\Users\gates\Personal\google-health-cli"
sys.path.insert(0, PY_REPO)

from google_health import cli  # noqa: E402
from google_health import config as cfgmod  # noqa: E402
from google_health import daily_log as logmod  # noqa: E402
from google_health import health  # noqa: E402

ROOT = pathlib.Path(__file__).resolve().parent.parent
FIXTURE = ROOT / "testdata" / "fixtures" / "exercise_all.json"
GOLDEN = ROOT / "testdata" / "golden"
GOLDEN.mkdir(parents=True, exist_ok=True)

POINTS = json.loads(FIXTURE.read_text(encoding="utf-8"))["dataPoints"]

CFG = {
    "elliptical_types": ["ELLIPTICAL"],
    "zone2_low": 110,
    "zone2_high": 130,
    "daily_log": "dummy",
}


def write_golden(name: str, text: str) -> None:
    # newline="" keeps LF endings on Windows (the goldens are LF; the Go tests
    # normalize \r before comparing).
    (GOLDEN / name).write_text(text, encoding="utf-8", newline="")
    print(f"wrote {name} ({len(text)} bytes)")


# ---- Stub the network + config so the real command paths run offline. --------
health.list_exercise_sessions = lambda cfg, start, end: POINTS
cfgmod.load_config = lambda: dict(CFG)

# GOAL.md §11 fix: zone-neutral session name. Patch the literal in-place so the
# rest of build_cardio_entry stays the real implementation.
_orig_build = logmod.build_cardio_entry


def _build_cardio_entry_fixed(sessions, cfg):
    entry = _orig_build(sessions, cfg)
    if entry is not None and entry.get("session") == "Zone 2 elliptical":
        entry["session"] = "Elliptical"
    return entry


logmod.build_cardio_entry = _build_cardio_entry_fixed
cli.logmod.build_cardio_entry = _build_cardio_entry_fixed


class Args:
    pass


def run(fn, **kw) -> str:
    a = Args()
    for k, v in kw.items():
        setattr(a, k, v)
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        fn(a)
    return buf.getvalue()


def gen_sessions() -> None:
    write_golden("sessions_json.golden",
                 run(cli.cmd_sessions, date="today", days=7, raw=False, json=True))
    write_golden("sessions_raw.golden",
                 run(cli.cmd_sessions, date="today", days=7, raw=True, json=False))


DAILY_LOG_INPUT = ROOT / "testdata" / "fixtures" / "daily_log_input.json"


def gen_daily_log() -> None:
    """Drive the REAL cmd_sync against a temp copy of the input doc and capture
    both the written DAILY_LOG.json (the #1 fidelity surface) and the human
    stdout. The exercise fixture provides the elliptical sessions; target/days
    are chosen so all four elliptical days fall in the window."""
    tmpdir = tempfile.mkdtemp()
    tmp = pathlib.Path(tmpdir) / "DAILY_LOG.json"
    shutil.copyfile(DAILY_LOG_INPUT, tmp)

    cfg = dict(CFG)
    cfg["daily_log"] = str(tmp)
    cfgmod.load_config = lambda: dict(cfg)

    human = run(cli.cmd_sync, date="2026-06-16", days=30, dry_run=False, json=False)
    write_golden("sync_human.golden", human)

    # The written file is CRLF on Windows (Python text-mode); normalize to LF for
    # the committed golden. The Go test compares Doc.Bytes() (LF) to it.
    written = tmp.read_bytes().replace(b"\r\n", b"\n").decode("utf-8")
    write_golden("daily_log_output.golden", written)
    shutil.rmtree(tmpdir, ignore_errors=True)


if __name__ == "__main__":
    gen_sessions()
    gen_daily_log()
    print("done")
