"""cardio-cli — log NordicTrack/elliptical Zone 2 cardio into DAILY_LOG.json.

The cardio counterpart to speediance-cli. Where speediance-cli pulls Steven's
strength sessions from the Speediance cloud and writes them into the WEEKS sheet,
this tool pulls his *cardio* sessions from the Google Health API (via the
`ghealth` binary) and writes them into DAILY_LOG.json.

Both his elliptical AND his Speediance strength sessions land in Google Health
(his watch logs everything), so the core job here is the DEDUP FILTER: keep only
elliptical/cross-trainer sessions and drop strength training, otherwise we would
double-count the strength work that speediance-cli already owns.
"""

__version__ = "0.1.0"
