# Impact scanner: what it finds and what it misses

See `docs/impact-accuracy.md` for measured precision and recall.

## What it finds
- Calls to the drifted endpoint, including through wrapper functions (e.g. `getJSON<User>(url)`), up to two levels deep. Wrappers can be plain functions, arrow functions in variables, object members (`api.load()`) or class methods (`svc.load()`).
- URLs written as string literals, template literals, simple concatenation, or held in a `const` variable, an object member or a small helper function (`fetch(API.user(9))`).
- Reads of the field through property access, optional chaining, string-key access (`x["field"]`), and object destructuring.
- Declarations of the field in interfaces, type aliases and Zod `z.object` shapes that are linked to the endpoint's response type.
- Fields read from values that came from the call: `await res.json()`, `.then(...)` chains, `.map` / `.filter` / `.find` callbacks, and `for...of` loops.
- Untyped data passed into a project function: the parameter is followed up to 3 calls deep.

## Confidence levels
- `type-linked`: the TypeScript type of the thing being read is the endpoint's response type (or a type nested in it).
- `flow-linked`: the value is traced from the endpoint call, but its type is untyped or `any`.
- `name-only`: only the field name matches. Might be an unrelated object. Hide with `--no-name-only`.

## Known misses and limits
- Dynamic access (`obj[key]` with a variable key) is not found.
- Values stored in React state or a store (`useState`, Redux `action.payload` typed `any`) and read later lose the flow link.
- Data typed `any` that is passed through more than 3 functions loses the flow link.
- React Query / SWR: found if the hook is given the response type (e.g. `useQuery<User>`) or the fetch wrapper is typed.
- Wrappers more than two levels deep are not followed.
- Only the last segment of a path is searched (`$.user.role_id` searches `role_id`); parent segments are not checked yet.
- URLs built from function parameters, environment variables or complex config objects are not matched.
- Non-TypeScript frontends (plain JS without types, Vue templates, Svelte) are not covered.
- Declarations from `z.infer` are not linked to the schema property.
- Later uses of a destructured local (e.g. `{user_name}` in JSX after `const { user_name } = user`) are not listed as separate lines; the destructuring line and its file are.