# Xivi Android TV

## Application and deployment contract

The Android APK has two entry points under `com.xivi.app`: the existing phone
activity and the native `.tv.TvActivity` Leanback launcher. The TV activity uses
Compose for TV, not Capacitor or a WebView. It supports API 28+ and targets API 36.
The first TV build is version **1.1.0 / versionCode 2**. Keep the existing external
release signing identity; a debug APK cannot update a release-signed installation.

Deploy the matching server first. Migration `000031` adds TV devices, short-lived
access tokens, pairing requests, proof replay evidence and viewer preferences.
`/healthz` must report `tv_api_version: 1`. Back up the database and the existing
server authentication key together: both are needed to preserve device grants.
Configure trusted reverse proxies correctly so DPoP sees the external HTTPS
scheme, host and path. Plain HTTP, self-signed certificates, alternate media
origins and redirects are deliberately rejected by the TV client.

No Android Auto, DVR, downloads, multiview, extended timeshift or neighboring
stream prewarming is introduced. TV playback stops and releases its viewer on
`onStop`; it does not use the phone foreground service or automatic PiP.

## Viewer experience

- Startup: last successfully rendered channel, guide, or a chosen channel.
- Guide: seven rows/two hours, compact ten rows/three hours, or large five
  rows/90 minutes. Fixed channel identity column, current-time marker, details,
  live preview, date selection, local-calendar day jumps and seven-day browsing.
- Mini-guide: three rows with current/next programmes; browsing starts no stream.
- Vertical movement retains the time anchor across uneven programme durations.
  Empty EPG cells remain selectable. Past/future selections show details and an
  explicit Watch Live action, never a recording or catch-up affordance.
- Filters: accessible lineup, group, named favorites and lineup-wide on-now
  categories. Typed channel/programme search supports current/all lineups.
- Favorites, hidden channels and ordering are account preferences with revision
  checks. The web viewer's **Lists** page (`/watch/preferences`) provides a
  phone-friendly editor. TV refreshes account preferences while foregrounded;
  conflicts reload current data and require the edit to be retried.
- Startup, display, buffering, track preferences, per-channel overrides and ten
  recent channels are local to a TV/server/account. Cache clearing, preference
  resets, history clearing and server-side sign-out are separate operations.

### Remote defaults

| Button         | Full-screen playback                                       | Guide / mini-guide                    |
| -------------- | ---------------------------------------------------------- | ------------------------------------- |
| Up / Down      | Browse channel banner; key release commits final selection | Previous / next row at the same time  |
| Left           | Previous successfully watched channel                      | Previous programme / time             |
| Right          | Mini-guide                                                 | Next programme / time                 |
| OK             | Quick controls, starting on Guide                          | Tune current; otherwise details       |
| Back           | Finish activity                                            | Close surface, restoring prior focus  |
| Guide          | Full guide                                                 | Filters (from full guide)             |
| Channel / Page | Surf                                                       | Page through rows                     |
| Numbers        | Tune on OK or 1.4-second timeout                           | Jump to channel                       |
| Info / Last    | Details / previous channel                                 | Contextual details / previous channel |

Quick controls expose all essential actions without a special remote button.
Left/right shortcuts can be reassigned. System Home, voice, volume and power are
not intercepted. First-use hints are dismissible; Controls Help remains available.
Text scaling follows Android plus the local preference. High contrast and reduced
motion are provided; navigation sounds follow Android's system policy.
Large font scales reduce visible guide rows and label the adaptation as
"Large text layout"; the toolbar remains horizontally scrollable by D-pad.

## Native boundaries and performance

`NativeApi` and `NativePlayback` are shared edges. `TvAuthRepository`,
`TvRepository`, `TvPlayer` and `TvViewModel` own TV identity, data, playback and
presentation respectively. `TvSettingsStore` uses DataStore; `TvCacheDatabase`
uses Room. No TV presentation types cross into the Svelte phone interface.

The guide composes visible rows/cells only. Metadata-only catalog pages load in
the background; the last channel and visible schedule do not wait for all pages.
Expired cursors restart without discarding selection. Room entries are scoped
by server/account, bounded to 256 entries / approximately 32 MiB, and pruned
after 14 days. Artwork is bounded to 4 MiB per image / 64 MiB on disk with a
12 MiB TV bitmap memory cache. Saved listings are labeled; response generation
time is not misrepresented as upstream EPG freshness.

One ExoPlayer and surface survive overlays and guide transitions. Each committed
tune receives a fresh UUID. Superseded jobs are cancelled, late callbacks are
identity-checked, and release requests precede new source requests. Temporary
release failures are retried and retained rather than forgotten. Playback URLs
contain playback/viewer identities, never credentials. HLS manifests and assets
use the same origin-constrained, DPoP-authenticated OkHttp client as artwork/API
requests. Token renewal is serialized without serializing media downloads.

Fast/Balanced/Stable startup thresholds are 500/1,000/2,000 ms, with rebuffer
thresholds twice those values. The actual live offset/window remains authoritative.
Frame-rate matching is off by default; the optional setting permits seamless
changes only. Audio/caption menus list delivered Media3 tracks, not hypothetical
provider tracks. Resuming beyond the live window explains the jump back to live.

Timing events use the existing telemetry endpoint: `tv_tune_request` at manifest
readiness, `tv_first_frame`, `tv_rebuffer`, `tv_tune_cancelled`, `tv_recovery`.
They carry a tune UUID, request timestamp, monotonic durations, manifest timing
and buffering preset. Events without an existing server stream incident can be
unavailable; controlled-source and real-provider measurements must be separated.

## Pairing and security

1. Enter and validate a trusted HTTPS origin on the TV.
2. Display a ten-minute, key-bound device request as a QR approval URL and an
   eight-character user code. The QR contains neither passwords nor device tokens.
3. The phone browser completes existing password/MFA/forced-password-change
   checks, previews the TV name/code and explicitly approves or rejects it.
4. Exchange the device code using a Keystore-generated P-256 signing key. The
   transaction stores the TV grant and encrypted retry delivery together so a
   lost first response does not create a second grant.
5. Renew 15-minute access tokens using an encrypted, opaque, non-expiring refresh
   credential. Renewal does **not** rotate the durable credential.

Both authenticated resources and renewal require ES256 DPoP. Resource proofs bind
the token hash, HTTP method and HTTPS URI; token requests additionally use a
server nonce. A database-backed JTI ledger rejects proof replay across processes
and server restarts. TV credentials are rejected as Bearer tokens. Private signing
keys stay in Android Keystore; refresh credentials use Keystore AES-GCM storage,
and successful encrypted persistence precedes the paired UI state. Cloud backup
and device transfer exclude device-bound credentials.

TV grants can use viewer endpoints only, even for an administrator account.
Approval retains browser CSRF/origin protections; only successfully authenticated
native grants bypass browser CSRF. Account/admin session pages list and revoke
TVs. Explicit revocation, disabled users, forced password changes and auth-version
changes invalidate grants. A missing key requires pairing again. Network/proxy,
server and clock failures retain credentials and show a disconnected state.
Normal restart, update or inactivity is not a logout condition.

The endpoints and response models are documented in `openapi-v2.yaml`, including
typed search, visible channel filtering, metadata-only catalogs, coverage,
revision-checked preferences and per-user device administration.

## Verification and release gates

### Local verification record — 2026-09-13

- Android JVM tests, debug lint, application APK and instrumentation APK builds
  passed. Lint has no errors; existing/dependency advisory warnings remain.
- All six TV instrumentation tests passed on an API 36.1 phone emulator with a
  1920×1080 landscape viewport. Standard, compact and large/high-contrast guide
  screenshots were inspected; this is not a substitute for Android TV hardware.
- The complete Docker `security-tests` target passed, including frontend checks,
  dependency audit, Go tests with native media dependencies, govulncheck and the
  configured gosec rules. The configured Semgrep scan reported no findings.
- Trivy 0.74.0 found no fixed HIGH/CRITICAL vulnerabilities in the locally built
  deployment image. An SPDX JSON SBOM was generated for that image. This scan
  used the existing `--ignore-unfixed` policy, not an assertion that every
  dependency is free of vulnerabilities.
- A versioned **debug** APK was produced. Release signing, installation over the
  previous release APK, hosted emulator CI, physical-TV testing, soak testing and
  percentile performance measurements have not been completed. Nothing was
  published or pushed.

### Reproducing checks

Development commands (use `.bat` on Windows):

```sh
npm ci
npm run check
npm run build:mobile
android/gradlew -p android :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
android/gradlew -p android :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.package=com.xivi.app.tv
docker build --target security-tests --progress plain .
```

Set `ANDROID_SERIAL` when multiple emulators/devices are connected. The TV test
package exercises real Android Keystore persistence, a simulated process restart,
concurrent token failures producing one renewal, temporary errors versus
revocation, HTTPS/origin restrictions, a 5,000-channel guide, time anchoring,
held-key browsing, rapid tunes, single-viewer accounting, decoder retention and
foreground-only playback. Its HTTP/media fixtures are local synthetic data, not
a provider or network-failover soak test. JVM tests cover guide focus, gaps,
overlap, DST, ordering, preferences and ES256 signature encoding. Backend tests
cover SQL grants/retry delivery, expiry, revocation, replay, preferences, typed
authorized search and the controller/middleware pairing boundary.

`android-tv.yml` adds pinned-action CI for an API 28 Android TV emulator and an
API 35 Google APIs emulator with TV-sized geometry, using JDK 21 and Node 24.
It builds both entry points, runs native unit/lint checks and executes the TV
instrumentation package. This workflow still needs its first hosted run after
the changes are pushed. Signing and publication are separate: see the manual,
reviewer-gated **Android release** workflow in [Android release](android-release.md).
It reuses both the native-TV and security verification workflows before signing.

Before releasing, complete and record these **hardware acceptance gates**:

- Android 9 device, low-memory TV, Google TV hardware and Shield; minimal D-pad
  plus channel/number-button remotes; full setup/search/settings without touch.
- TalkBack, large system fonts, all density presets, high contrast, reduced motion,
  focus during refresh, missing/overlapping EPG, midnight/DST and page boundaries.
- Single-connection provider, rapid A→B→C surfing, genuine source failover,
  access-token expiry during HLS, Wi-Fi loss/recovery and long foreground soak.
- No audio/PiP after Home/Back exit; actual language/caption delivery; pause outside
  the live window; no false DVR or prewarming behavior.
- Pairing across reboot, prolonged inactivity, signed updates and lost responses;
  revocation from another device; copied-token and proof replay rejection.
- Cached input/banner p95 <100 ms, cached guide open p95 <250 ms. For a controlled
  healthy one-second-keyframe source: warm tune p95 <2 s, cold p95 <5 s. These are
  release targets, not measured guarantees for arbitrary sources.
- Full security CI, including Semgrep and final-image Trivy/SBOM in addition to
  the Docker test target. Do not bypass dependency/security failures.

For release, configure `XIVI_KEYSTORE_PATH`, `XIVI_KEYSTORE_PASSWORD`,
`XIVI_KEY_ALIAS`, `XIVI_KEY_PASSWORD` through the existing secure environment,
then build `:app:assembleRelease`. Do not put these values or the keystore in Git.
Verify the APK certificate with `apksigner verify --print-certs`, retain its
SHA-256, and install with `adb install -r` over the prior **release-signed** APK.
Confirm both pairing and preferences survive before distributing the versioned
APK. This implementation does not add an updater or publish to a store.

Emulator tests and debug APKs are development evidence, not a signed/hardware-
accepted release. Never uninstall or clear data as an update test: that correctly
destroys the Keystore-bound enrollment.

## Standards and platform references

- [Compose for TV](https://developer.android.com/training/tv/playback/compose)
- [Android TV navigation](https://developer.android.com/training/tv/get-started/navigation)
- [Media3 live streaming](https://developer.android.com/media/media3/exoplayer/live-streaming)
- [Device authorization interaction](https://www.rfc-editor.org/rfc/rfc8628.html)
- [DPoP](https://www.rfc-editor.org/rfc/rfc9449.html)
- [Android Keystore](https://developer.android.com/privacy-and-security/keystore)
