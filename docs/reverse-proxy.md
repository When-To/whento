# Running WhenTo behind a reverse proxy

WhenTo speaks plain HTTP on port 8080 and terminates no TLS of its own. Any
public deployment therefore sits behind a reverse proxy, and that proxy has to be
configured for three things: TLS, server-sent events, and — the one that is
silently wrong in most setups — `TRUSTED_PROXIES`.

Start with §1. It is the part that decides whether your rate limiting works at
all.

---

## 1. `TRUSTED_PROXIES`: the setting that makes rate limiting real

### Why it is now the only mechanism

WhenTo deliberately does **not** use chi's `RealIP` middleware. `RealIP`
overwrites `RemoteAddr` from `X-Forwarded-For` / `X-Real-IP` for *every* request,
before anything can ask whether the connection came from a proxy at all — which
means any client can pick its own rate-limit bucket by sending a header. The
comment in [`cmd/main.go`](../cmd/main.go) states it plainly: it is absent and
must stay absent.

Proxy headers are honoured in exactly one place instead —
`middleware.IPKeyFunc` in [`pkg/middleware/ratelimit.go`](../pkg/middleware/ratelimit.go) —
and only when the connection itself originates from an address listed in
`TRUSTED_PROXIES`. So:

- **`TRUSTED_PROXIES` empty (the default):** `X-Forwarded-For` and `X-Real-IP`
  are ignored, always. Behind a proxy, every request looks like it comes from the
  proxy's address, so **all of your users share one rate-limit bucket**. Login is
  limited to 5 requests per minute *for the entire instance*, and one enthusiastic
  visitor locks everyone out.
- **`TRUSTED_PROXIES` too wide** (`0.0.0.0/0`, or a range that includes clients):
  any request can claim any IP through a header, and per-IP limits become
  decorative.
- **`TRUSTED_PROXIES` set to exactly the address your proxy connects from:**
  correct.

### What value to use

The value must be the source address **as the application container sees it**,
which is not the same thing as where the proxy runs.

| Where the proxy runs                                                    | What the app sees as the peer                    | Set `TRUSTED_PROXIES` to                             |
| ----------------------------------------------------------------------- | ------------------------------------------------ | ---------------------------------------------------- |
| A container on the same Compose network (`whento-network`)              | The proxy container's IP on that bridge network  | The bridge subnet, e.g. `172.18.0.0/16`              |
| On the Docker host, proxying to the published port `127.0.0.1:8080`     | The bridge gateway address                       | That gateway, e.g. `172.18.0.1/32`                   |
| On another machine                                                       | That machine's address                           | Its IP, e.g. `10.0.0.7/32`                           |
| A CDN in front of your own proxy                                         | Still your proxy                                 | Your proxy — **not** the CDN's ranges (see below)    |

Both forms are accepted: bare IPs (`10.0.0.7`) and CIDR ranges
(`172.18.0.0/16`, `2001:db8::/32`), comma-separated.

### Hostnames are accepted, and resolved once

A Compose service name (`nginx`, `traefik`, `caddy`) is also accepted. It is the
name that identifies the proxy everywhere else in a compose file, so it is the
one people reach for here — and left unresolved it would be stored verbatim and
compared against a numeric peer address, which never matches. WhenTo therefore
resolves it **at startup** and trusts every address it gets back (`localhost` is
both `127.0.0.1` and `::1`, and both are trusted).

Two consequences, and they are why an IP or a CIDR range is still the
recommended value:

- **The address is frozen for the life of the process.** Recreating the proxy
  container gives it a new address on the bridge network; WhenTo goes on trusting
  the old one until it is restarted, and until then you are back in the
  everyone-shares-one-bucket case above with nothing in the logs to say so. A
  startup warning names the entry every time one is resolved.
- **A name that does not resolve stops the startup.** At that point it is a typo,
  and accepting it would reproduce exactly the silent failure this is meant to
  avoid.

Find the real value rather than guessing:

```bash
# The Compose network's subnet and gateway
docker network inspect whento_whento-network \
  -f '{{range .IPAM.Config}}subnet={{.Subnet}} gateway={{.Gateway}}{{end}}'

# The proxy container's address on it, if the proxy is containerised
docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' my-caddy
```

Docker bridge subnets are stable for the life of a network but are reassigned if
the network is recreated. Prefer the subnet form over a single container IP, and
prefer an explicitly declared subnet in your compose file if you want it fixed
forever.

You cannot read the answer out of the application's logs: WhenTo does not log
client IP addresses, on purpose (see
[logging-and-privacy.md](logging-and-privacy.md)).

### Which entry of `X-Forwarded-For` is used

The **last** one. `IPKeyFunc` takes `parts[len(parts)-1]` — the entry closest to
the proxy that added it — falling back to `X-Real-IP` if the header is absent.

This is why the standard proxy configurations below are safe: both Caddy and
nginx's `$proxy_add_x_forwarded_for` *append* the address of the peer they
actually accepted the connection from. A client that sends
`X-Forwarded-For: 1.2.3.4` only prepends a lie; the real address is still the
last element.

It also means a CDN in front of your proxy will be the last entry, and every
visitor will share the CDN edge's bucket. If you run one, have your proxy
overwrite the header with the CDN's client-IP header (`CF-Connecting-IP`,
`True-Client-IP`, …) after validating that the request really came from the CDN's
published ranges.

### Verifying

```bash
curl -s -D- -o /dev/null https://your-domain.com/api/health
```

Look at `X-RateLimit-Remaining` on a rate-limited endpoint from two different
client machines: with `TRUSTED_PROXIES` set correctly, each one has its own
budget; with it unset, they draw down a single shared counter.

---

## 2. Caddy

Caddy gets and renews TLS certificates on its own, which makes it the shortest
correct configuration.

```caddyfile
your-domain.com {
    # Caddy appends the client address to X-Forwarded-For and sets
    # X-Forwarded-Proto, which is what WhenTo needs for rate limiting and for
    # emitting HSTS. Nothing extra to configure for either.
    reverse_proxy whento-app:8080 {
        # Server-sent events: /api/v1/availabilities/calendar/{token}/events is a
        # long-lived stream. Without this, Caddy's read timeout ends it and every
        # browser reconnects in a loop.
        transport http {
            read_timeout 0
        }
        flush_interval -1
    }
}
```

`flush_interval -1` disables response buffering, so each `event: update` reaches
the browser when it is written rather than when a buffer fills.

If Caddy runs on the Docker host instead of in the stack's network, use
`reverse_proxy 127.0.0.1:8080` and set `TRUSTED_PROXIES` to the bridge gateway.

Full stack, Caddy as a container on the same network:

```yaml
# docker-compose.override.yml
services:
  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    networks:
      - whento-network

volumes:
  caddy_data:
  caddy_config:
```

With that override, drop the `ports:` mapping from the `app` service so the
application is reachable only through Caddy.

---

## 3. nginx

```nginx
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    # WhenTo sets its own security headers (CSP, HSTS, frame options,
    # Permissions-Policy). Do not add duplicates here: two CSP headers are
    # intersected by the browser and the result is rarely what either of you
    # meant.

    # The application rejects request bodies above 1 MB itself; matching that
    # here means the rejection is cheap.
    client_max_body_size 1m;

    location / {
        proxy_pass http://127.0.0.1:8080;

        proxy_set_header Host              $host;
        # Appends $remote_addr, so the LAST entry is the peer nginx really saw.
        # That is the entry WhenTo reads.
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        # Required for WhenTo to emit Strict-Transport-Security.
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host  $host;

        proxy_http_version 1.1;
    }

    # Server-sent events: live availability updates.
    location /api/v1/availabilities/ {
        proxy_pass http://127.0.0.1:8080;

        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_http_version 1.1;
        # A stream must not be buffered, and must not be cut off after 60s.
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
    }
}

server {
    listen 80;
    listen [::]:80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}
```

The SSE endpoint already sends `X-Accel-Buffering: no`, which nginx honours, so
the dedicated `location` block is belt and braces — useful because that header
only helps if nothing else in the chain re-buffers the response.

Then, on the app:

```bash
# .env — nginx on the Docker host, app reached through the published port
TRUSTED_PROXIES=172.18.0.1/32
```

---

## 4. Checklist

- [ ] TLS terminated at the proxy, HTTP redirected to HTTPS.
- [ ] `X-Forwarded-Proto` set, so WhenTo emits HSTS.
- [ ] `X-Forwarded-For` appended (not overwritten with a client-supplied value).
- [ ] `TRUSTED_PROXIES` set to the address the proxy connects *from*, verified
      with `docker network inspect`, and as narrow as you can make it. A service
      name works but is resolved only at startup — prefer an IP or a CIDR range.
- [ ] `APP_URL` set to the public HTTPS URL — it is what goes into emails.
- [ ] `CORS_ORIGINS` set to the same public origin (it defaults to `APP_URL`).
- [ ] Passkeys: `WEBAUTHN_RP_ID` / `WEBAUTHN_RP_ORIGIN` are derived from
      `APP_URL` when unset, so a wrong `APP_URL` breaks passkey registration
      with an origin mismatch rather than a visible error.
- [ ] SSE not buffered and not timed out.
- [ ] The `app` container's port no longer published directly to the internet.
- [ ] Rate limiting verified from two different client addresses.
