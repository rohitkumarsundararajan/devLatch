# Dashboard backend (devlatch ui)

`devlatch ui` runs the normal proxy and a local dashboard API together:

    devlatch ui --target http://localhost:8000 [--port 9000] [--ui-port 7070]

Point your app at the proxy port. If an approved contract exists, every new
difference between it and live traffic is published as a `drift` event.
Without a contract it only learns endpoints.

Both servers bind to 127.0.0.1 only, and the API rejects any Host header that
is not loopback (DNS-rebinding protection). There is no remote exposure.

## Endpoints

| Method and path | Returns |
| --- | --- |
| `GET /api/health` | `{ok, version}` |
| `GET /api/state` | version, learned endpoints with response counts, last event id, drift event count |
| `GET /api/drifts?limit=N` | recent drift events (newest last, max 500) |
| `GET /api/contract` | the approved contract file as JSON (503 if none) |
| `GET /api/events` | live Server-Sent Events stream |

## Event stream

Events have an `id`, an `event` type and JSON `data`:

- `status`: `{state, target, contract}` once at start
- `endpoint`: `{key}` the first time an endpoint is seen
- `drift`: a violation, same fields as `check --format json` (endpoint, path,
  type, severity, rename_from, rename_to, confidence, evidence)

A client that reconnects with `Last-Event-ID` (or `?last_id=N`) first gets the
events it missed (the last 500 are kept in memory), then live ones. Each drift
is reported once per run. A slow client may miss events and recover by
reconnecting. The proxy is never slowed by the dashboard.

## Actions

| Method and path | Does |
| --- | --- |
| `GET /api/impact?endpoint=&path=` | frontend files and lines that read the drifted field (needs `--frontend`) |
| `GET /api/fix/preview?endpoint=&path=&to=` | the rename as before and after lines, nothing is written |
| `POST /api/fix/apply` | writes the rename into the frontend (body: endpoint, path, to) |
| `POST /api/approve` | accepts live traffic as the contract (body: `{"endpoint": "..."}`, empty means every endpoint with live traffic) |

Impact and fix need `--frontend <folder>` (or `--tsconfig`) and the scanner;
without them the endpoints answer 501 and `GET /api/state` reports
`actions.impact` and `actions.fix` as false.

Requests that change files (the two POST endpoints) must send
`Content-Type: application/json`, an `Origin` that matches the host (if the
browser sends one) and the header `X-DevLatch-Token`. The token is random per
run and only readable from `GET /api/state`, which another website cannot read,
so a page in your browser cannot rewrite your files. A successful action is
published on the stream as an `action` event.

Approving writes `.devlatch/contracts.json` (commit it to git) and clears the
drift already reported for those endpoints.

## Desktop notifications

`devlatch ui` shows an OS notification when breaking drift is found, at most
one every 10 seconds. Use `--no-notify` to turn it off. If the system cannot
show one (for example Linux without a notification service), a single warning
is printed and everything else keeps working.
