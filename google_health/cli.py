"""google-health-cli — pull elliptical cardio from Google Health into DAILY_LOG.json.

Commands:
  doctor     Check ghealth is reachable and authenticated.
  sessions   List recent exercise sessions (all types) so we can see how
             elliptical vs strength is tagged. Use --raw to dump the API JSON.
  sync       Pull recent sessions, keep ONLY elliptical, write to DAILY_LOG.json.

stdout stays parseable with --json; human hints go to stderr.
"""
import sys
import json
import argparse
from datetime import date, datetime, time, timedelta

from . import config as cfgmod
from . import health
from . import daily_log as logmod


def _err(*a):
    print(*a, file=sys.stderr)


def _resolve_date(s):
    if not s or s == "today":
        return date.today()
    if s == "yesterday":
        return date.today() - timedelta(days=1)
    return datetime.strptime(s, "%Y-%m-%d").date()


def _window(target, days):
    """[start, end) covering `days` calendar days ending on `target`, as naive
    local civil datetimes — the exercise endpoint filters on civil start time.
    """
    start = datetime.combine(target - timedelta(days=days - 1), time.min)
    end = datetime.combine(target + timedelta(days=1), time.min)
    return start, end


def cmd_doctor(args):
    cfg = cfgmod.load_config()
    info = health.run_ghealth(cfg, ["doctor"])
    print(json.dumps(info, indent=2))
    if not info.get("tokenValid"):
        _err("\nNot authenticated yet. Run:  ghealth auth login")
        sys.exit(2)


def cmd_sessions(args):
    cfg = cfgmod.load_config()
    target = _resolve_date(args.date)
    start, end = _window(target, args.days)
    points = health.list_exercise_sessions(cfg, start, end)

    if args.raw:
        print(json.dumps(points, indent=2))
        return

    rows = [health.parse_session(p) for p in points]
    rows = [r for r in rows if r.get("start")]
    rows.sort(key=lambda r: r["start"])
    out = [{
        "date": r["start"].date().isoformat() if r["start"] else None,
        "exercise_type": r["exercise_type"],
        "elliptical": health.is_elliptical(r, cfg),
        "duration_min": r["duration_min"],
        "avg_hr": r["avg_hr"],
        "calories": r["calories"],
        "platform": r["platform"],
    } for r in rows]

    if args.json:
        print(json.dumps(out, indent=2))
        return
    if not out:
        print(f"No exercise sessions found in the last {args.days} day(s).")
        return
    print(f"{len(out)} session(s) in the last {args.days} day(s) "
          f"(* = counts as cardio, others ignored):\n")
    for r in out:
        mark = "*" if r["elliptical"] else " "
        hr = f"{r['avg_hr']} avg" if r["avg_hr"] else "no HR"
        print(f"  {mark} {r['date']}  {str(r['exercise_type']):<18} "
              f"{str(r['duration_min'] or '?'):>3} min  {hr:>7}  {r['platform'] or ''}")


def cmd_sync(args):
    cfg = cfgmod.load_config()
    target = _resolve_date(args.date)
    start, end = _window(target, args.days)
    points = health.list_exercise_sessions(cfg, start, end)
    all_sessions = [health.parse_session(p) for p in points]

    # DEDUP FILTER: elliptical only. Strength (speediance-cli's job) is dropped.
    cardio = [s for s in all_sessions if s.get("start") and health.is_elliptical(s, cfg)]
    dropped = len(all_sessions) - len(cardio)

    # Bucket the kept sessions by their LOCAL start date.
    by_day = {}
    for s in cardio:
        day_iso = s["start"].date().isoformat()
        by_day.setdefault(day_iso, []).append(s)

    # Only touch days within the requested window (target back `days`).
    wanted = {(target - timedelta(days=i)).isoformat() for i in range(args.days)}
    by_day = {d: v for d, v in by_day.items() if d in wanted}

    if not by_day:
        print(f"No elliptical sessions to log for the last {args.days} day(s) "
              f"ending {target}. ({dropped} non-cardio session(s) ignored.)")
        return

    doc = logmod.load(cfg["daily_log"])
    summary = []
    for day_iso in sorted(by_day):
        entry = logmod.build_cardio_entry(by_day[day_iso], cfg)
        if args.dry_run:
            summary.append((day_iso, entry, "dry-run"))
            continue
        _, status = logmod.upsert_cardio(doc, day_iso, entry)
        summary.append((day_iso, entry, status))

    wrote_any = any(s in ("created", "updated") for _, _, s in summary)
    if not args.dry_run and wrote_any:
        logmod.save(cfg["daily_log"], doc)

    tag = "[dry-run] would write" if args.dry_run else "synced"
    print(f"{tag} {len(summary)} cardio day(s); {dropped} non-cardio ignored.\n")
    FLAG = {"created": "  (new day)", "updated": "  (updated)", "dry-run": "",
            "conflict": "  !! SKIPPED - manual session already logged this day"}
    for day_iso, entry, status in summary:
        print(f"  {day_iso}: {entry['duration_min']} min, "
              f"avg HR {entry['avg_hr']} [{entry['zone2']}], "
              f"{entry['calories']} kcal{FLAG.get(status, '')}")


def main(argv=None):
    p = argparse.ArgumentParser(prog="google_health",
                                description="Sync Google Health data (elliptical cardio) into DAILY_LOG.json.")
    sub = p.add_subparsers(dest="cmd", required=True)

    sp = sub.add_parser("doctor", help="Check ghealth setup + auth")
    sp.set_defaults(func=cmd_doctor)

    sp = sub.add_parser("sessions", help="List recent exercise sessions (all types)")
    sp.add_argument("--date", default="today", help="today | yesterday | YYYY-MM-DD")
    sp.add_argument("--days", type=int, default=7)
    sp.add_argument("--raw", action="store_true", help="dump raw ghealth JSON")
    sp.add_argument("--json", action="store_true", help="machine-readable output")
    sp.set_defaults(func=cmd_sessions)

    sp = sub.add_parser("sync", help="Write elliptical sessions into DAILY_LOG.json")
    sp.add_argument("--date", default="today", help="today | yesterday | YYYY-MM-DD")
    sp.add_argument("--days", type=int, default=3)
    sp.add_argument("--dry-run", action="store_true", help="show what would be written")
    sp.set_defaults(func=cmd_sync)

    args = p.parse_args(argv)
    try:
        args.func(args)
    except health.GHealthError as e:
        _err(f"ghealth error: {e}")
        sys.exit(2)


if __name__ == "__main__":
    main()
