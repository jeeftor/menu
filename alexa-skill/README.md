# Alexa Skill: School Lunch Menu

This directory holds the Alexa skill configuration for the `menu` server. The
skill backend is an HTTPS endpoint served directly by the existing Go binary
(`menu serve`) at `/alexa` — no separate AWS Lambda is required.

## How it works

```text
Alexa cloud → HTTPS menu.vookie.net/alexa
            → Cloudflare (bypass policy for menu.vookie.net)
            → Caddy → menu serve /alexa
            → validates Amazon's request signature
            → looks up the lunch menu
            → replies with natural speech
```

The menu site is intentionally public for reading (calendar, menus, Alexa).  Writes
(settings, food-images, favorites, exclusions, section-includes) are restricted
to the LAN by the Go middleware via the `Cf-Connecting-Ip` header.

Amazon signs every request with a certificate chain and timestamp. The Go server
verifies that signature before answering, so the `/alexa` path does **not**
need Authentik authentication.

## Files

- `skill.json` — skill manifest. Endpoint is `https://menu.vookie.net/alexa`.
- `interactionModels/custom/en-US.json` — voice model with the
  `MenuQueryIntent` and sample utterances.
- `../internal/alexa/` — Go package that parses ASK requests, verifies
  signatures, and builds responses.

## Server flags

```bash
menu serve \
  --alexa-skill-id amzn1.ask.skill.xxxxxxxx \
  --alexa-school woodmen-roberts-elementary-school \
  --alexa-meal lunch \
  --external-url https://menu.vookie.net
```

| Flag | Purpose |
|------|---------|
| `--alexa-skill-id` | Your Alexa skill application ID. Enables `/alexa`. |
| `--alexa-disable-verification` | Skip Amazon signature checks. **Local testing only.** |
| `--alexa-school` | Default school slug (matches `nutrislice.DefaultSchools`). |
| `--alexa-meal` | `lunch` or `breakfast`. |
| `--oidc-issuer` | OIDC issuer URL (e.g. `https://auth.vookie.net/application/o/menu/`). |
| `--oidc-client-id` | OIDC client ID from Authentik. |
| `--oidc-client-secret` | OIDC client secret from Authentik. |
| `--oidc-redirect-url` | Callback URL, e.g. `https://menu.vookie.net/callback`. |
| `--session-secret` | Base64 secret (32+ bytes) used to sign session cookies. |

## Reverse proxy / auth changes

The menu site uses the Go server's own LAN/WAN detection instead of Caddy
`wan_auth` or Authentik.  The Caddy block is a plain reverse proxy with an
app-native auth marker:

```caddy
# Menu / recipe planner
@menu host menu.vookie.net
handle @menu {
    # auth: app-native (WAN read-only; LAN or OIDC writes via Cf-Connecting-Ip detection)
    reverse_proxy http://10.0.0.112:13383 {
        import proxy_headers
    }
}
```

### Access model

- **Reads** (calendar, menus, `/api/v1/lunch/*`, Alexa) are public.
- **LAN / Tailscale** traffic has full access to settings and config endpoints.
- **WAN** traffic can optionally log in via Authentik OIDC to manage settings
  remotely.  Logged-in users see the settings gear and can POST config changes.
- When OIDC is not configured, settings remain LAN-only from the internet.

### Cloudflare Access

Create a Self-hosted Access app named `Bypass CF (menu.vookie.net)` covering
`menu.vookie.net` with a single **Bypass** policy (same pattern as `ai.vookie.net`,
`searxng.vookie.net`, etc.).  This lets the menu site and Alexa reach the LAN
without a Cloudflare login; the Go middleware still blocks writes from WAN.

### Authentik OIDC setup

Run the Authentik setup script in the `homelab` repo:

```bash
ssh root@dockarr 'docker exec authentik-server ak shell -c "$(cat arr/authentik/scripts/16_menu_oidc.py)"'
```

It prints the **Client ID** and **Client Secret**.  Pass them to `menu serve`:

```bash
menu serve \
  --oidc-issuer https://auth.vookie.net/application/o/menu/ \
  --oidc-client-id <client-id> \
  --oidc-client-secret <client-secret> \
  --oidc-redirect-url https://menu.vookie.net/callback \
  --session-secret <base64-32-byte-secret>
```

Generate a session secret with: `openssl rand -base64 32`.

## Alexa Developer Console setup

1. Create a new Custom skill.
2. Under **Build → Interaction Model**, upload or paste the contents of
   `interactionModels/custom/en-US.json`.
   - Invocation name: `menu`
3. Under **Build → Endpoint**:
   - Service endpoint type: **HTTPS**
   - Default region: `https://menu.vookie.net/alexa`
   - SSL certificate type: **Trusted** (Cloudflare provides the public cert)
4. Find the skill ID under **Build → Skill IDs** and pass it to
   `--alexa-skill-id`.
5. Save and build the model.

## Testing

Local, no verification:

```bash
go run main.go serve \
  --alexa-skill-id test \
  --alexa-disable-verification \
  --cache-dir /tmp/menu-cache

curl -X POST http://localhost:8080/alexa \
  -H 'Content-Type: application/json' \
  -d '{
    "version": "1.0",
    "request": {
      "type": "IntentRequest",
      "requestId": "amzn1.echo-api.request.test",
      "timestamp": "2026-08-18T11:00:00Z",
      "locale": "en-US",
      "intent": {
        "name": "MenuQueryIntent",
        "slots": {"date": {"name": "date", "value": "tomorrow"}}
      }
    }
  }'
```

Live Alexa test:

- Deploy the Caddyfile change and restart Caddy (`make pull restart` on
  `10.0.0.15`).
- Start `menu serve` with your real skill ID and verification enabled.
- In the Alexa Developer Console, use the **Test** tab or an Echo device.
- Say: *"Alexa, ask Menu what's for lunch tomorrow."*

## What can be asked

- "What's for lunch?"
- "What's for lunch tomorrow?"
- "What's on the menu for Monday?"
- "What are they serving today?"

The `date` slot uses `AMAZON.DATE`, so relative references like today,
tomorrow, and weekdays work automatically.
