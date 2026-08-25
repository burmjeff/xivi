## XIVI

### ToDo

- [ ] Fix all the things

### DEV ENV

1. https://code.visualstudio.com/docs/devcontainers/containers
2. https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers

### RUN

1. Install onnxruntime
2. Setup: npm run setup-all
3. Rename `.env.example` to `.env` and fill it with your environment values.
4. Build it: npm run build-all
5. Run it: npm run serve
6. Go to your API Docs page: [127.0.0.1:3000/swagger/index.html](http://127.0.0.1:3000/swagger/index.html)

### Virtual tuner device outputs

Enable **Tuner** on an individual lineup in Studio's Device outputs panel to advertise only that lineup as a virtual network tuner. Each enabled lineup receives a stable device ID and its own `discover.json`, `lineup.json`, and MPEG-TS stream URLs. Plex, Emby, and Jellyfin can therefore add the lineups independently.

Discovery uses the native network-tuner broadcast protocol on UDP port `65001`. A Linux Docker deployment should use host networking so LAN broadcasts reach Xivi:

```sh
docker run --network host \
  --log-driver local --log-opt max-size=10m --log-opt max-file=3 \
  -v xivi-serve:/xivi/serve -v xivi-config:/xivi/config xivi
```

Xivi writes application logs to stdout/stderr rather than duplicating them in
its data volume. The bounded Docker logging options above prevent the container
runtime's logs from growing without limit. Configure an equivalent rotation
policy when using another container orchestrator.

Publishing only `65001/udp` through Docker's bridge does not reliably forward LAN broadcast traffic. When auto-discovery is unavailable, copy a lineup's **Tuner** address from Device outputs and add that address manually in the media server.
