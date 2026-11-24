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

## Not here yet

The browser UI (P11), approve-from-UI, impact and fix views and OS alerts (P12).