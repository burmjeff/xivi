## XIVI

### ToDo

- [ ] Fix all the things

### DEV ENV

1. https://code.visualstudio.com/docs/devcontainers/containers
2. https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers

### Secure production cutover

Internet-facing installations require authentication, HTTPS through a trusted reverse proxy, an externally mounted authentication key, explicit trusted-network CIDRs, and replacement media-key URLs for players. Follow [the secure deployment and recovery guide](docs/security-deployment.md) before upgrading a populated instance.

There is no public registration. On the first server start Xivi creates a temporary `xivi` / `xivi` administrator. It is accepted through any normally permitted Xivi transport, including public HTTPS, and opens only the required password-change screen; Watch, Studio, streams, images, MFA, sessions, and device access remain locked until it is replaced. Change it immediately. Administrators manage viewers and lineup grants in Studio → Users. Users create revocable, reusable M3U/XMLTV links in Account → Device access.

### RUN

1. Install onnxruntime
2. Setup: npm run setup-all
3. For production, create a root-owned environment file using the variables in
   [the secure deployment guide](docs/security-deployment.md). Development can
   continue to use the defaults in `config.yaml.example`.
4. Build it: npm run build-all
5. Run it: npm run serve
6. Go to your API Docs page: [127.0.0.1:3000/swagger/index.html](http://127.0.0.1:3000/swagger/index.html)

### Shared streaming and source limits

When stream proxying is enabled, Xivi opens one GStreamer producer per active
channel and shares it across MPEG-TS and HLS viewers. Configure **Maximum stream
connections** for each playlist in Studio → Sources. Cold starts, retries,
make-before-break recovery, and optional channel prewarming all consume the same
per-source budget and never exceed it. A limit of `1` disables parallel recovery
against that source; ordered variants from a different source can still be
prepared when their own budget permits.

Studio → Streams shows current source-budget use and per-session media,
connections, bitrate, failovers, and errors. Studio → Settings → Streaming
controls the startup race, recovery hedge, prewarm pool, and browser HLS
compatibility mode. The compatibility mode transcodes only codecs browsers
commonly reject and leaves the canonical MPEG-TS branch unchanged.

### Virtual tuner device outputs

Enable **Tuner** on an individual lineup in Studio's Device outputs panel to advertise only that lineup as a virtual network tuner. Each enabled lineup receives a stable device ID and rotatable signed credential for its own `discover.json`, `lineup.json`, and MPEG-TS stream URLs. Plex, Emby, and Jellyfin can therefore add the lineups independently.

Discovery uses the native network-tuner broadcast protocol on UDP port `65001`
and only answers clients inside `TRUSTED_LAN_CIDRS`. A Linux Docker deployment
should use host networking so LAN broadcasts reach Xivi:

```sh
docker run --network host \
  --user 10001:10001 --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --log-driver local --log-opt max-size=10m --log-opt max-file=3 \
  --env-file /srv/xivi/xivi.env \
  --mount type=bind,src=/srv/xivi/secrets/auth.key,dst=/run/secrets/xivi-auth.key,readonly \
  -v /srv/xivi/serve:/xivi/serve \
  -v /srv/xivi/config:/xivi/config xivi
```

Xivi writes application logs to stdout/stderr rather than duplicating them in
its data volume. The bounded Docker logging options above prevent the container
runtime's logs from growing without limit. Configure an equivalent rotation
policy when using another container orchestrator.

Publishing only `65001/udp` through Docker's bridge does not reliably forward LAN broadcast traffic. When auto-discovery is unavailable, copy a lineup's **Tuner** address from Device outputs and add that address manually in the media server.
