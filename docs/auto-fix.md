# Auto-fix: `devlatch fix`

When the API renames a field, `devlatch fix` rewrites the frontend code that reads it. It never writes anything unless you ask.

## Use

~~~
devlatch check -format json > check.json
devlatch fix -frontend path/to/frontend -from-check check.json          # preview (writes nothing)
devlatch fix -frontend path/to/frontend -from-check check.json -apply   # write the changes
devlatch fix -frontend path/to/frontend -from-check check.json -pr      # branch, commit, push, open a PR (needs git and gh)
~~~

Or name one rename yourself:

~~~
devlatch fix -frontend path/to/frontend -endpoint "GET /api/v1/users/{id} 200" -path '$.user_name' -to username -apply
~~~

## What it changes

- Property reads, including optional chaining: `user.user_name` becomes `user.username`
- String-key reads: `user["user_name"]`
- Destructuring: `{ user_name }` becomes `{ username: user_name }`, so the rest of the function keeps working
- Declarations of the field in interfaces, type literals and Zod shapes linked to the endpoint
- `Pick<User, "user_name">` and `Omit<User, "user_name">` keys

Only usages the scanner links to the endpoint (`type-linked` and `flow-linked`) are changed. Name-only matches are skipped unless you pass `-include-name-only`.

## Safety

- Preview is the default. `-apply` writes; `-pr` creates a branch, so your current branch is not touched.
- After writing, the scanner type-checks the frontend with its `tsconfig.json`. If the number of TypeScript errors went up, every file is restored and the command fails.
- `-pr` refuses to run on a dirty working tree (use `-allow-dirty` to override).
- Always review the diff and run your tests before merging.

## What it does not change

- Only simple renames: same parent object, new last name. A field that moved to a different parent is reported as skipped.
- Object literals that build values (mock data, request bodies), `keyof` and `Record` keys, dynamic access (`obj[key]`), and non-TypeScript files.
- Usages the scanner could not link to the endpoint (see `docs/impact-limitations.md`).

Exit codes: 0 ok (including a preview), 2 error or reverted.