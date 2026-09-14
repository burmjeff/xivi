<p align="center">
  <img src="static/brand/signal-tile.svg" alt="" width="80" height="80" />
</p>

<h1 align="center">Xivi</h1>

<p align="center">
  <strong>Your channels. One guide.</strong><br />
  Self-hosted live TV for the web, Android, and Android TV.
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#your-first-lineup">First lineup</a> ·
  <a href="#android">Android</a> ·
  <a href="#development">Development</a>
</p>

---

Xivi brings your M3U playlists and XMLTV programme guides together. Organize channels in **Studio**, watch them in **Watch**, and share your lineups with compatible players.

Bring your own sources: Xivi does not include TV channels, subscriptions, or programme listings. Only connect content you are authorized to access.

## What you can do

- **Build your guide.** Combine sources, match programme listings, and arrange channels into groups and ordered lineups.
- **Find something to watch.** Browse the guide, search channels and programmes, and manage favorites.
- **Manage playback.** Share an upstream stream across viewers, set source connection limits, and configure ordered failover.
- **Control access.** Create viewer accounts, grant access per lineup, enable MFA, and revoke sessions or player links.
- **Connect your screens.** Use the browser, native Android playback, or M3U/XMLTV and virtual-tuner outputs.

| Where you watch         | Experience                                                                        |
| ----------------------- | --------------------------------------------------------------------------------- |
| Web browser             | Live playback, guide, search, and the administrator's Studio                      |
| Android phone or tablet | Shared Watch interface with native playback and picture-in-picture                |
| Android TV / Google TV  | Native, remote-friendly guide and phone-assisted pairing                          |
| Compatible players      | Authenticated M3U/XMLTV links; optional virtual tuner for Plex, Emby, or Jellyfin |

**Status:** actively developed. Android clients are in preview; real-device and signed-update validation are still in progress. Xivi is live-first: no built-in DVR, downloads, or offline playback.

## Quick start

Use a **Linux x86-64 Docker host**, or Docker Desktop running Linux containers. The current server image targets `linux/amd64`; other architectures require emulation. No separate database server is needed.

The commands below use a Bash-compatible shell and start a **local-only** instance on the Docker host.

### 1. Build the server

```sh
git clone https://github.com/burmjeff/xivi.git
cd xivi
docker build --platform linux/amd64 -t xivi:local .
```

The first build downloads the frontend, Go, and native media dependencies.

### 2. Start Xivi

```sh
docker run -d --name xivi \
  --restart unless-stopped \
  --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --log-driver local --log-opt max-size=10m --log-opt max-file=3 \
  -p 127.0.0.1:3000:3000 \
  -e LOCAL_BASE_URL=http://127.0.0.1:3000 \
  -v xivi-config:/xivi/config \
  -v xivi-serve:/xivi/serve \
  xivi:local
```

### 3. Sign in

Open [http://127.0.0.1:3000](http://127.0.0.1:3000) on that computer. On a fresh installation, sign in with username **`xivi`** and password **`xivi`**, then immediately choose a new password. Playback and administration stay locked until you do.

> **Before connecting other devices or exposing Xivi to the internet:** follow the [secure deployment guide](docs/security-deployment.md). The example above deliberately listens only on loopback. Android requires a reachable HTTPS server with a trusted certificate; it cannot use this localhost-only setup.

## Your first lineup

1. **Studio → Sources:** add an M3U URL. Set **Maximum stream connections** to the limit allowed by your provider.
2. **Studio → Guide data:** add your XMLTV source, if available.
3. **Studio → Lineups:** create a lineup, add channels or source groups, arrange their order, and select **Publish** to generate its outputs.
4. Open **Watch** to browse and play. To share access, create viewers and assign their lineups in **Studio → Users**.

For an external player, create an entry in **Account → Device access** and copy its M3U/XMLTV links. Treat those links like passwords; revoke them when a device is no longer yours.

For a virtual tuner, enable **Tuner** in a lineup's **Device outputs**. Automatic discovery needs LAN broadcast access on UDP `65001`, which generally means Linux host networking. Otherwise, add the lineup's tuner address manually. See the [networking guidance](docs/security-deployment.md#3-start-the-hardened-container).

## Keep your data safe

The quick start creates two [Docker named volumes](https://docs.docker.com/engine/storage/volumes/), which persist when the container is replaced:

| Volume        | Contents                                       |
| ------------- | ---------------------------------------------- |
| `xivi-config` | Configuration, SQLite database, and `auth.key` |
| `xivi-serve`  | Generated outputs, artwork, and working caches |

Stop Xivi and back up both volumes before upgrading. Keep the matching `auth.key` with your protected backups: losing it makes encrypted provider credentials and MFA secrets unrecoverable. Do not delete the volumes when updating.

To check an installation:

```sh
docker logs --tail 100 xivi
docker exec xivi /xivi/xivi healthcheck
```

Use the [deployment and recovery guide](docs/security-deployment.md) for HTTPS, trusted proxies, storage migration, backups, and administrator recovery. Never expose port `3000` directly to the public internet.

## Android

**One APK serves phones, tablets, and TVs.** Each launcher opens the appropriate interface. The app supports Android 9+ and connects to your own Xivi server over trusted HTTPS.

- **Phone/tablet:** enter your server address and sign in.
- **TV:** enter the server address, then approve pairing on your phone using the QR code or short code. Pairing persists until revoked or app data/device keys are lost.
- **Updates:** install a newer APK signed with the same key over the existing app. Uninstalling or clearing app data removes local sign-in and preferences.

To build a development APK, install Node.js 24, Android Studio, and Android SDK Platform 36. Gradle selects the project's configured JDK automatically. From the repository root:

```sh
npm ci
npm run build:mobile
cd android
./gradlew assembleDebug
```

On Windows, use `.\gradlew.bat assembleDebug` for the last command. The APK is written to `android/app/build/outputs/apk/debug/`. Debug builds are for development, not a replacement for a release-signed installation.

For remote controls, pairing, and testing details, see the [Android TV guide](docs/android-tv.md).

## Development

The web interface uses **SvelteKit**; the server uses **Go, SQLite, GStreamer, libvips, and ONNX Runtime**. Android adds **Kotlin, Media3, and Compose for TV**.

The included [development container](.devcontainer/devcontainer.json) supplies the server's native dependencies. Open the repository in VS Code and choose **Dev Containers: Reopen in Container**; setup installs the project dependencies.

Build and run the complete web/server application inside that container:

```sh
npm run build-all
npm run serve
```

For frontend hot reload, leave the server running and use `npm run dev` in another terminal, then open port `5173`. The Vite development server forwards API requests to port `3000`.

Run the frontend check with `npm run check`. Run the Docker-backed server tests and dependency audits from the repository root:

```sh
docker build --platform linux/amd64 --target security-tests \
  --no-cache-filter security-tests .
```

The [security workflow](.github/workflows/security.yml) adds static analysis, final-image vulnerability scanning, and an SBOM. The current API contract is [OpenAPI v2](docs/openapi-v2.yaml); administrators can also open `/docs/` on their running server.

## Feedback

Found a bug or have an idea? [Open an issue](https://github.com/burmjeff/xivi/issues) with your version, device/browser, and steps to reproduce.

Redact passwords, provider URLs, cookies, tokens, and device-access links from logs and screenshots. Do not attach databases or signing keys, and do not disclose security vulnerabilities in public issues; report those privately to the maintainer.

## License

Xivi is licensed under [GNU AGPLv3 only](LICENSE). You can use, modify, and redistribute it, including commercially, subject to the license's source-sharing requirements. It comes without warranty.

Third-party components retain their [own licenses](THIRD_PARTY_NOTICES.md). See [licensing and distribution](LICENSING.md) for source availability, releases, and contributions.
