# Secure deployment and recovery

Xivi's authenticated release is a security cutover. Back up the database and
configuration first, keep the authentication key outside the database, and
replace every old player URL after the upgrade. Old anonymous API, playlist,
guide, image, stream, and tuner URLs intentionally stop working.

## 1. Back up and prepare storage

Stop Xivi before copying its SQLite database so the backup is consistent. Back
up both `/srv/xivi/config` and `/srv/xivi/serve` to access-controlled storage.
The authentication key must be backed up separately after it is created. Losing
that key makes encrypted provider URLs and TOTP seeds unrecoverable.

Create the writable directories for Xivi's dedicated UID, then restrict access:

```sh
sudo install -d -m 0700 -o 10001 -g 10001 /srv/xivi/config /srv/xivi/serve
sudo install -m 0600 -o root -g root /dev/null /srv/xivi/xivi.env
```

Older Xivi images ran as root. Before reusing storage from one of those images,
stop the old container and transfer both mounted trees to the new runtime UID:

```sh
sudo chown -R 10001:10001 /srv/xivi/config /srv/xivi/serve
sudo chmod 0700 /srv/xivi/config /srv/xivi/serve
```

For Docker named volumes, run the equivalent one-time ownership migration with
the new image (replace the volume names when necessary):

```sh
docker run --rm --user 0 --entrypoint /bin/chown \
  -v config:/xivi/config -v serve:/xivi/serve xivi \
  -R 10001:10001 /xivi/config /xivi/serve
```

On first startup Xivi atomically creates a 256-bit root key at
`/xivi/config/auth.key` with mode `0600`. It never replaces an existing key, so
every restart reuses the same value from the persistent config volume. Set
`XIVI_AUTH_KEY_FILE` only when an externally managed key path is required; a
missing configured path is created when its parent is writable.

Never store the key in the image, Compose file, Git, SQLite database, logs, or a
support bundle. A restore requires the matching database and authentication key.

## 2. Configure the network boundary

For a public HTTPS deployment, put these values in `/srv/xivi/xivi.env`,
replacing every example address:

```dotenv
XIVI_PRODUCTION=true
PUBLIC_BASE_URL=https://tv.example.com
PUBLIC_MEDIA_BASE_URL=https://media.example.com
LOCAL_BASE_URL=http://192.168.1.10:3000
TRUSTED_PROXY_CIDRS=192.168.1.20/32
# Alternatively, for a shared user-defined Docker network:
# TRUSTED_PROXY_HOSTS=reverse-proxy
TRUSTED_LAN_CIDRS=192.168.1.0/24
ALLOW_LAN_HTTP=true
SERVER_HOST=0.0.0.0
SERVER_PORT=3000
```

`PUBLIC_BASE_URL` and `PUBLIC_MEDIA_BASE_URL` are optional. Leave them unset
for a local-only production container; Xivi will use `LOCAL_BASE_URL` and does
not require a reverse proxy in that mode. When either public URL is configured,
one of `TRUSTED_PROXY_CIDRS` or `TRUSTED_PROXY_HOSTS` is required so forwarded
HTTPS and client-address headers can only be accepted from the actual proxy.
`TRUSTED_LAN_CIDRS` is optional. If it is empty, directly connected loopback,
RFC1918, and IPv6 ULA clients are trusted automatically. Adding any LAN CIDR
replaces that automatic behavior with the explicit allowlist.

`TRUSTED_PROXY_CIDRS` contains only the direct IP/CIDR of the reverse proxy,
not client networks. `TRUSTED_PROXY_HOSTS` accepts Docker service names or DNS
names that resolve to private or loopback addresses from inside Xivi. Hostname
results refresh every 15 seconds and fail closed when resolution fails. This is
useful when Xivi and the proxy share a user-defined Docker network, where the
service name remains stable while the container address changes. Public DNS
results are intentionally ignored; use an explicit CIDR for a non-private proxy
peer. Never use a hostname supplied by a request or reverse DNS as a trust signal.

Xivi ignores forwarded scheme and client-IP headers from any other peer. A
request carrying proxy-forwarding headers is never granted automatic LAN trust,
even when its direct Docker or LAN peer address is private. Every reverse proxy
must therefore be listed explicitly. `TRUSTED_LAN_CIDRS` contains only networks
permitted to use LAN sessions, LAN media keys, and tuner discovery. Set
`ALLOW_LAN_HTTP=false` when local HTTPS is available.

Bind the direct port only to the LAN interface and block it at the public
firewall. Public traffic must reach Xivi through the configured reverse proxy and
HTTPS. The proxy should redirect ordinary public HTTP traffic to HTTPS and add HSTS,
but a credential-bearing `/media/`, stream, HLS, logo, or tuner request must not
be redirected; reject it instead so its query credential cannot leak.

Configure the reverse proxy to overwrite forwarded client/scheme headers with values it
observes, and configure its access logger to omit URL query strings, redact the
credential-bearing portion of `/m/*` and `/x/*`, and omit `Cookie`,
`Authorization`, and `X-CSRF-Token` headers. Full media keys necessarily travel
in generated compatibility-client query strings, while short output aliases are
path credentials; ordinary proxy logs must never become a second credential store.

`PUBLIC_MEDIA_BASE_URL` is optional. When it is configured, route both public
hostnames to the same Xivi port but apply different edge policy:

- the application hostname carries browser and API traffic and may use WAF bot
  scoring or an adaptive browser challenge;
- the media hostname carries key-authenticated M3U, XMLTV, images, HLS, and
  MPEG-TS and must never present an HTML challenge to Plex, Jellyfin, Emby, VLC,
  or another compatibility client;
- rate-limit new media connections at the edge only as an additional coarse
  safeguard. Do not rate-limit individual HLS segments.

Xivi independently applies cost-weighted request budgets to network, address,
account/session, and device-key identities. It also limits genuinely new stream
starts and concurrent downstream viewers. Configure the latter controls under
**Studio → Settings → Streaming**. Large result pages and guide windows consume
more budget, while ordinary HLS refreshes do not consume a stream-start slot.

If the reverse proxy performs rate limiting, confirm that the feature is present
in the deployed build. Keep any proxy administration endpoint bound to loopback
or a protected local socket.

Direct LAN HTTP is a compatibility mode, not confidential transport. Anyone
able to observe that LAN can read passwords and page data. Prefer local TLS.

## 3. Start the hardened container

The production image runs as UID/GID `10001:10001`. Keep the root filesystem
read-only, drop Linux capabilities, and bound container logs:

```sh
docker run --name xivi --network host \
  --user 10001:10001 --read-only --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --log-driver local --log-opt max-size=10m --log-opt max-file=3 \
  --env-file /srv/xivi/xivi.env \
  -v /srv/xivi/serve:/xivi/serve \
  -v /srv/xivi/config:/xivi/config xivi
```

Host networking is needed for automatic tuner broadcast discovery. If that is
not required, publish the application port only on the selected LAN address.
Do not publish the container directly on a public interface. Docker service-name
resolution requires Xivi and the reverse proxy to share a user-defined network;
it is not available with the host-network example above. Use an explicit proxy
CIDR when running Xivi with host networking.

On first startup Xivi applies additive migrations and encrypts stored provider
URLs. It fails closed in production when the authentication key cannot be
created or read, when a configured public deployment has no trusted proxy CIDR
or hostname, or when a configured network identity is invalid.

## 4. Complete the first-run password change and verify access

On the first server start, an empty user database receives one temporary
administrator with username `xivi` and password `xivi`. Open Xivi directly at
its configured public HTTPS or trusted-LAN address, sign in, and replace that
password with an 8–128 character password or passphrase. The temporary
credential is accepted through every normally permitted Xivi transport, while
its session can reach only session lookup, sign-out, and password change.
Watch, Studio, streams, images, MFA, session management, and media keys remain
locked until the password is replaced.

The anonymous bootstrap-status endpoint exposes detailed first-run guidance
only to direct trusted-LAN clients. Public callers receive a neutral response,
so the endpoint does not advertise that the temporary administrator is active.

Complete this password change immediately after first startup, then enroll TOTP
MFA. In Studio, create viewer accounts and grant only their intended lineups.
Verify that a viewer cannot open Studio or enumerate another lineup.

Create named device access from **Account → Device access** after password
reauthentication. Xivi never displays the standalone credential; readable
`/m/ABCD-EFGH-JKMN-PQRS` and `/x/ABCD-EFGH-JKMN-PQRS` output aliases remain on
the device entry until it is revoked or expires. Most players need only the M3U
address because it embeds the corresponding XMLTV address.
Replace every saved M3U/XMLTV player URL with the generated public or LAN URL;
old bare URLs are not supported. The credential used in those links is retained
encrypted under `auth.key`, while request authentication continues to use its
keyed hash.
Use the per-lineup Device outputs control to rotate a tuner credential.

Before exposing the service, verify:

- unauthenticated Watch, Studio, API, media, image, and stream requests fail;
- viewer lineup grants apply to search, guide, playback, logos, and telemetry;
- public keys fail on direct untrusted HTTP and LAN keys fail through the reverse proxy;
- disabling an account or revoking a key disconnects its downstream streams;
- logs and diagnostics contain no session, CSRF, provider, or media secrets.

## Recovery and key handling

Reset a lost password or disable a lost MFA factor through the local interactive
CLI. Both operations revoke browser sessions; password reset also revokes media
keys.

```sh
docker exec -it xivi /xivi/xivi auth reset-password
docker exec -it xivi /xivi/xivi auth disable-mfa
```

Do not replace `auth.key` as an ordinary password-reset step. To rotate it,
decrypt/re-encrypt all recoverable secrets in a controlled migration and revoke
all sessions and media keys. Restoring an older database without its matching
key will fail startup rather than silently discarding encrypted data.

For rollback, stop Xivi and restore the pre-cutover database, configuration,
serve directory, and corresponding image as one unit. Never run old code against
a database whose provider URLs have already been encrypted by the cutover.

## Security maintenance

Treat Internet exposure as an ongoing process:

1. Review and merge dependency updates at least monthly and promptly for known
   exploited or critical issues.
2. Require the security workflow on every pull request. It runs Go and Svelte
   tests, `npm audit`, `govulncheck`, SQL-focused `gosec` and Semgrep rules,
   a final-image Trivy scan, and SBOM generation.
3. Rebuild the image from the pinned Dockerfile after dependency or base-image
   updates; do not patch a running container by hand.
4. Retain the SBOM with each release and scan the exact published image digest.
5. Review security audit events, disabled accounts, active sessions, media-key
   last-use data, trusted proxy identities, and reverse-proxy access controls on
   a regular schedule.
6. Exercise database-and-key restoration and local admin recovery before an
   emergency. Report suspected credential exposure by revoking the affected
   sessions/media keys immediately and rotating upstream provider credentials.

Revoked and expired device-access keys are retained for the configured security
retention period (90 days by default), then deleted by the startup maintenance
pass or the recurring storage cleanup (every six hours by default). Deletion
cascades to lineup grants; SQLite secure deletion and bounded compaction prevent
the encrypted credential rows from accumulating indefinitely.

No deployment is vulnerability-proof. Keep the host, Docker runtime, reverse proxy,
browser clients, media servers, and Xivi dependencies patched, and repeat an
authenticated application scan after material authorization or streaming
changes.
