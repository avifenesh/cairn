---
name: agent-browser
description: "Browse the web using Agent Browser Protocol (ABP). Use when asked to open a website, interact with a web page, click buttons, fill forms, take screenshots, or automate browser tasks. Keywords: browse, visit, open page, click, screenshot, web page, navigate, fill form, scrape."
argument-hint: "[url-or-instruction]"
allowed-tools: "cairn.shell"
---

# Agent Browser Protocol (ABP)

Deterministic browser control via REST API. Chromium build that freezes JS between steps — every action returns a settled screenshot + events.

**Server**: `http://localhost:8222` | **Base**: `/api/v1`

## Start ABP

```bash
curl -sf http://localhost:8222/api/v1/browser/status && echo "ABP running" || {
  npx -y agent-browser-protocol --headless -- --no-sandbox &
  sleep 15 && echo "ABP started"
}
```

Headless servers need `--headless` + `--no-sandbox` (Ubuntu 23.10+ AppArmor). First run downloads Chromium (~15s).

## Workflow

1. Start ABP (above)
2. List tabs: `curl -s http://localhost:8222/api/v1/tabs | jq '.[0].id' -r` → save as TAB_ID
3. Navigate to $ARGUMENTS or requested URL
4. Inspect screenshot → decide next action → act → repeat

## Navigate

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/navigate \
  -H 'content-type: application/json' \
  -d '{"url":"$ARGUMENTS","screenshot":{"markup":"interactive"}}'
```

Also: `back`, `forward`, `reload` (POST, same path pattern, no body needed).

## Click

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/click \
  -H 'content-type: application/json' \
  -d '{"x":450,"y":320,"screenshot":{"markup":"interactive"}}'
```

## Type

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/type \
  -H 'content-type: application/json' -d '{"text":"hello world"}'
```

Key press: `POST .../keyboard/press` with `{"key":"Enter"}`. Clear field: `POST .../clear-text`.

## Scroll

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/scroll \
  -H 'content-type: application/json' \
  -d '{"x":640,"y":360,"deltaY":300}'
```

deltaY: positive = down, negative = up.

## Tabs

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs \
  -H 'content-type: application/json' -d '{"url":"https://example.com"}'
curl -s -X DELETE http://localhost:8222/api/v1/tabs/{TAB_ID}
```

Note: `POST /tabs` creates + navigates but does NOT return screenshots. Call `navigate` after for screenshots.

## Dialogs

```bash
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/dialog/accept
curl -s -X POST http://localhost:8222/api/v1/tabs/{TAB_ID}/dialog/dismiss
```

## Reading pages

All action endpoints (navigate, click, scroll, back, forward) return a JSON envelope with `screenshot_after.data` (base64 webp). To view:

```bash
# Pipe action response through python to save screenshot
... | python3 -c "
import sys,json,base64
d=json.load(sys.stdin)
raw=base64.b64decode(d['screenshot_after']['data'])
open('/tmp/page.webp','wb').write(raw)
from PIL import Image
Image.open('/tmp/page.webp').save('/tmp/page.png')
print(f'Saved {len(raw)} bytes')
"
```

Then read `/tmp/page.png` to see the page. Crop for detail: `img.crop((0,0,1280,400)).save('/tmp/top.png')`.

Note: `text` and `execute` endpoints return 405 in v0.1.6 — rely on screenshots.

## Response envelope

Every action returns: `screenshot_before`/`screenshot_after` (base64 webp), `scroll` (position + page dimensions), `events[]` (navigation, dialog, file_chooser, download_started, popup), `timing`, `cursor` (x, y, type).

## Key facts

- Page freezes between actions — no race conditions, no waits
- Always add `"screenshot":{"markup":"interactive"}` to see clickable/typeable elements highlighted
- Markup options: `interactive` (preset), or array: `clickable`, `typeable`, `scrollable`, `grid`, `selected`
- Viewport: 1280x800. Port: 8222
- GL/Vulkan errors in logs are harmless (headless GPU fallback)

## Shutdown

```bash
curl -s -X POST http://localhost:8222/api/v1/browser/shutdown
```
