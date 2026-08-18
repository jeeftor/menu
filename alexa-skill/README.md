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

## Reverse proxy / auth changes

The menu site uses the Go server's own LAN/WAN detection instead of Caddy
`wan_auth` or Authentik.  The Caddy block is a plain reverse proxy with an
app-native auth marker:

```caddy
# Menu / recipe planner
@menu host menu.vookie.net
handle @menu {
    # auth: app-native (WAN read-only; LAN-only writes via Cf-Connecting-Ip detection)
    reverse_proxy http://10.0.0.112:13383 {
        import proxy_headers
    }
}
```

### Cloudflare Access

Create a Self-hosted Access app named `Bypass CF (menu.vookie.net)` covering
`menu.vookie.net` with a single **Bypass** policy (same pattern as `ai.vookie.net`,
`searxng.vookie.net`, etc.).  This lets the menu site and Alexa reach the LAN
without a Cloudflare login; the Go middleware still blocks writes from WAN.

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
