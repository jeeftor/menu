# Alexa Skill: School Lunch Menu

This directory holds the Alexa skill configuration for the `menu` server. The
skill backend is an HTTPS endpoint served directly by the existing Go binary
(`menu serve`) at `/alexa` — no separate AWS Lambda is required.

## How it works

```text
Alexa cloud → HTTPS menu.vookie.net/alexa
            → Caddy → Authentik bypass (just for /alexa)
            → menu serve /alexa
            → validates Amazon's request signature
            → looks up the lunch menu
            → replies with natural speech
```

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

Because Alexa cannot complete an Authentik login flow, `menu.vookie.net/alexa`
must reach the Go server without going through `wan_auth`. The Caddy block for
`menu.vookie.net` needs a path-level bypass:

```caddy
@menu host menu.vookie.net
handle @menu {
    # Alexa skill endpoint authenticates via Amazon request signatures,
    # so it cannot go through Authentik forward_auth.
    handle /alexa* {
        reverse_proxy http://10.0.0.112:13383 {
            import proxy_headers
        }
    }
    import wan_auth
    reverse_proxy http://10.0.0.112:13383 {
        import proxy_headers
    }
}
```

### Cloudflare Access

`menu.vookie.net` is a Pattern H service (`wan_auth`). If your Cloudflare Zero
Trust setup has an Access app covering `*.vookie.net` or `menu.vookie.net`,
Alexa traffic will be blocked before it reaches Caddy. In that case you need a
bypass policy for `menu.vookie.net/alexa` in the Cloudflare dashboard
(**Zero Trust → Access → Applications**), similar to the existing bypasses for
`auth.vookie.net` or `kb.vookie.net`.

If `menu.vookie.net` is already exposed without CF Access (only `wan_auth`
protects it), no Cloudflare change is needed.

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
