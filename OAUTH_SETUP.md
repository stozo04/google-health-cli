# Google Health API — OAuth setup (one time, ~10 min)

Goal: let `ghealth` read **your own** Google Health data (the elliptical sessions
your watch logs) using read-only OAuth. You create a personal Google Cloud
project, enable the Google Health API, and authorize it for your own account in
**Testing** mode. No fees, no app review for personal use.

> Use the **same Google account that owns your watch data**
> (`gates.steven@gmail.com`) everywhere below.

## 1. Create a project
1. Go to <https://console.cloud.google.com/>.
2. Project dropdown (top bar) → **New Project** → name it `ghealth-personal` → **Create**.
3. Make sure that project is selected.

## 2. Enable the Google Health API
1. <https://console.cloud.google.com/apis/library> → search **"Google Health API"**.
2. Open it → **Enable**.
   - If it won't enable / says you need access or verification to *enable* it →
     **stop and tell Claude** (this is the one scenario we flagged as a risk).

## 3. Configure the consent screen (Google Auth Platform)
1. **APIs & Services → OAuth consent screen** (newer console: **Google Auth Platform → Branding/Audience**).
2. User type: **External** → **Create**.
3. App name: `ghealth-personal`. User support email + developer email: your email.
4. Save through the steps. On the **Audience** / publishing screen, leave
   **Publishing status = Testing**.
5. **Test users → Add users →** `gates.steven@gmail.com` → Save.
   (Testing mode + you as a test user is what lets you authorize the restricted
   health scopes without a production review.)

## 4. Create an OAuth client
1. **APIs & Services → Credentials → Create Credentials → OAuth client ID**.
2. Application type: **Desktop app**  ← important (gives a client secret +
   loopback redirect, which ghealth needs).
3. Name it `ghealth-cli` → **Create**.
4. Copy the **Client ID** and **Client secret** (or download the JSON).

## 5. Point ghealth at the client + log in
In PowerShell:
```powershell
$g = "C:\Users\gates\Personal\ghealth\ghealth.exe"
& $g config set client-id     "PASTE_CLIENT_ID"
& $g config set client-secret "PASTE_CLIENT_SECRET"
& $g auth login          # read-only scopes by default — do NOT add --write
```
`auth login` opens a browser:
1. Sign in as `gates.steven@gmail.com`.
2. You'll see **"Google hasn't verified this app."** This is expected — it's your
   own project. Click **Advanced → Go to ghealth-personal (unsafe)**.
3. Grant the read-only Google Health permissions.

## 6. Confirm
```powershell
& $g doctor      # expect  "tokenValid": true
```
Then tell Claude — everything after this runs locally on this machine.

---

### If consent is blocked even as a test user
If step 5 says the app must be verified before you (a test user) can consent,
that's the residual risk we flagged. Options, in order: (a) re-check you're
listed under **Test users** and logging in as that exact account; (b) confirm
the scopes shown are the `googlehealth.*` read scopes; (c) tell Claude — we'd
weigh whether the limited verification is worth it or fall back to manual cardio
logging. We won't force it.
