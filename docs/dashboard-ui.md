# Dashboard UI

Run `devlatch ui --target http://localhost:8000` and open http://127.0.0.1:7070.
The page and the API are served from the same local port, so there is nothing
to configure and nothing is exposed beyond your machine.

- **Live**: learned endpoints with response counts, and a drift feed that
  updates over Server-Sent Events (no refresh needed).
- **Contract**: the approved contract as an expandable field tree. Fields with
  drift are highlighted. Optional fields are marked with `?`.

## Working on the UI

The source is in `web/` (Vite, React, TypeScript). The built files are
committed in `internal/webui/dist` and embedded into the binary with go:embed,
so `go install` needs no Node.

    cd web
    npm install
    npm run dev      # http://localhost:5173, proxies /api to 127.0.0.1:7070
    npm run build    # writes internal/webui/dist, commit the result

Run `devlatch ui` in another terminal so the dev server has an API to talk to.
A test fails if the committed build is missing or is only the placeholder.