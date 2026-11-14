# Impact scanner: what it finds and what it misses (thin slice, Phase 4)

## What it finds
- Calls to the drifted endpoint, including through one or two levels of wrapper functions (e.g. `getJSON<User>(url)`).
- Reads of the field through property access, optional chaining, string-key access (`x["field"]`), and object destructuring.
- Declarations of the field in interfaces, type aliases and Zod `z.object` shapes that are linked to the endpoint's response type.
- Fields read from values that came from the call: `await res.json()`, `.then(...)` chains, `.map` / `.filter` / `.find` callbacks.

## Confidence levels
- `type-linked`: the TypeScript type of the thing being read is the endpoint's response type (or a type nested in it).
- `flow-linked`: the value is traced from the endpoint call, but its type is untyped or `any`.
- `name-only`: only the field name matches. Might be an unrelated object. Hide with `--no-name-only`.

## Known misses and limits
- Dynamic access (`obj[key]` with a variable key) is not found.
- Data typed `any` that is passed through several functions loses the flow link.
- React Query / SWR: found only if the hook is given the response type (e.g. `useQuery<User>`) or the fetch wrapper is typed.
- Aliased wrappers more than two levels deep are not followed.
- Only the last segment of a path is searched (`$.user.role_id` searches `role_id`); parent segments are not checked yet.
- The URL must be a string literal, a template literal, or a simple concatenation. URLs built from variables or config objects are not matched.
- Non-TypeScript frontends (plain JS without types, Vue templates, Svelte) are not covered.
- Declarations from `z.infer` are not linked to the schema property.

These limits are measured and tightened in Phase 8 (full scanner).
- Later uses of a destructured local (e.g. {user_name} in JSX after const { user_name } = user) are not listed as separate lines; the destructuring line and its file are.
