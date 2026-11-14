# DevLatch

**Zero-config API contract guard.** DevLatch learns your API's real response shapes from live traffic,
catches drift before it ships, tells you **which frontend files will break**, and generates the fix.

> Status: **Phase 0** (skeleton). Nothing user-facing works yet. The full plan is in
> [`devlatch_idea.md`](devlatch_idea.md); build progress is tracked in its checklist (section 23).

## Try the Phase 0 demo

Two terminals:

```bash
# 1. demo backend on http://127.0.0.1:8000
make demo-backend

# 2. demo frontend on http://localhost:5173 (proxies /api to :8000)
make demo-frontend-install   # first time only
make demo-frontend
```

Break the API on demand (these are the drift cases DevLatch must catch):

```bash
curl -X POST   localhost:8000/__demo/mutations/rename_user_name   # user_name -> username
curl -X POST   localhost:8000/__demo/mutations/balance_to_string  # balance 104.5 -> "104.50"
curl -X DELETE localhost:8000/__demo/mutations/balance_to_string  # undo one
curl -X POST   localhost:8000/__demo/reset                        # undo all
curl           localhost:8000/__demo/mutations                    # list available + active
```

| Mutation | Drift it simulates | Expected DevLatch result |
|---|---|---|
| `rename_user_name` | field rename | BREAKING (possible rename) |
| `balance_to_string` | NUMBER -> STRING | BREAKING |
| `tags_to_objects` | array of strings -> array of objects | BREAKING |
| `remove_email` | required key removed | BREAKING |
| `nullable_nickname` | field becomes nullable | WARNING |
| `add_currency` | new key | SAFE |
| `new_map_date` | date-keyed map gains a date | no violation (MAP) |

You can also start the backend pre-broken: `go run ./examples/demo-backend -mutate rename_user_name`.

## Development

```bash
make help     # all targets
make check    # gofmt + vet + tests with the race detector (what CI runs)
make build    # bin/devlatch
```

Requires Go 1.22+ and Node 20+ (Node only for the demo frontend and, later, the impact scanner).
Before publishing, replace the placeholder module path `github.com/OWNER/devlatch` (see `go.mod`).
