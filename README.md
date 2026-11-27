# DevLatch

Zero-config API contract guard. DevLatch watches real traffic between your frontend and backend, learns the response shapes, and fails your build when the API drifts (a field renamed, removed, or its type changed) before the frontend breaks.

- Learns a contract from live traffic through a local proxy, or by probing a public API directly
- Detects removed fields, type changes, and likely renames (POSSIBLE_RENAME with a confidence score and evidence)
- Lists the frontend files that read a drifted field (TypeScript)
- Exit codes for CI: 0 ok, 1 breaking drift, 2 error
- GitHub Action that comments on PRs, plus a scheduled watch that opens an issue on drift

## Install

Download a binary for your OS from the GitHub Releases page (devlatch_VERSION_OS_ARCH), or build from source (Go 1.22+):

~~~
go build -o devlatch ./cmd/devlatch
devlatch version
~~~

## Quickstart (about 5 minutes, no backend needed)

Probe a public API, save its contract, then check it:

~~~
devlatch init -target https://jsonplaceholder.typicode.com
devlatch probe -base https://jsonplaceholder.typicode.com -paths /posts/1,/users/1 -record
devlatch approve
devlatch probe -base https://jsonplaceholder.typicode.com -paths /posts/1,/users/1
~~~

The last command prints OK and exits 0. If a field in the API is renamed or removed, it exits 1 and says what changed.

## Using it with your own backend

~~~
devlatch init -target http://localhost:8000
devlatch record -duration 2m      # proxy on 127.0.0.1:9000: send your frontend or tests through it
devlatch approve                  # accept the recorded proposal as the contract
devlatch check -duration 30s      # later: exit 1 on breaking drift
~~~

Replay without a running backend: run record with -save-traffic traffic.jsonl, then devlatch check -from traffic.jsonl.

Useful flags: -format text|json|markdown, -fail-on breaking|warning, -renames=false (report REMOVED_KEY + ADDED_KEY instead of POSSIBLE_RENAME), -ignore /health,/metrics.

## Which frontend files break? (impact)

Needs Node.js and the scanner/ folder from this repo (run npm install inside scanner/ once):

~~~
devlatch impact -frontend path/to/frontend -endpoint "GET /api/v1/users/{id}" -path '$.user_name'
devlatch check -format json | devlatch impact -frontend path/to/frontend -from-check -
~~~

Accuracy: docs/impact-accuracy.md. Limitations: docs/impact-limitations.md. Rename detection: docs/rename-detection.md.

## Fix the frontend after a rename (fix)

~~~
devlatch check -format json > check.json
devlatch fix -frontend path/to/frontend -from-check check.json          # preview
devlatch fix -frontend path/to/frontend -from-check check.json -apply   # write
devlatch fix -frontend path/to/frontend -from-check check.json -pr      # branch + commit + PR (needs git and gh)
~~~

It rewrites reads, destructuring and type declarations of the renamed field, then re-runs the TypeScript check and restores everything if errors increased. Details: docs/auto-fix.md.
## CI
- Pull requests: .github/workflows/devlatch-pr.yml (uses the composite action in action/) runs devlatch check and posts a sticky comment. A breaking drift fails the check.
- Scheduled watch: .github/workflows/api-drift-watch.yml probes an API every 6 hours and opens, updates or closes a devlatch-drift issue.

## Commands

~~~
init      create .devlatch/config.json for a backend
record    learn a contract proposal from live traffic
approve   accept the recorded proposal as the contract
check     compare live traffic with the contract (exit 1 on breaking drift)
diff      compare two contract files (devlatch diff OLD NEW)
impact    list frontend files that read a drifted field
fix       rename drifted fields in frontend code (preview, --apply, --pr)
probe     GET an API directly (no proxy) to record or check its contract
watch     run a proxy and print the shapes it learns (no files written)
version   print the version
~~~

Run "devlatch COMMAND -h" for flags.

## License

MIT
## Impact and fix need Node

devlatch impact and devlatch fix use the TypeScript scanner in the scanner/ folder (included in release archives). Install Node 22 or newer, then run 
pm ci inside scanner/ once.

