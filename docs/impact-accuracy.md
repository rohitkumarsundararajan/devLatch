# Impact scanner accuracy

`devlatch impact` lists the frontend files that read a drifted API field. This page says how accurate it is, how that was measured, and what the numbers do not prove.

## How it is measured

Two small labeled TypeScript projects live in `scanner/fixtures/`. Every line that really uses an API field is marked with a `// @hit <case>` comment. The benchmark runs the scanner on them and compares line by line:

- Precision = found lines that are real usages / all found lines
- Recall = real usages that were found / all real usages

Two views are reported, because the scanner labels each hit with a confidence level:

- **Confident hits**: `type-linked` and `flow-linked` only. This is what `devlatch impact --no-name-only` shows.
- **All hits**: also includes `name-only` matches (same field name, but the scanner could not link it to the endpoint). These are the ones that can be wrong.

Run it yourself from `scanner/`:

~~~
npm run bench           # benchmark set (24 usages)
npm run bench:heldout   # held-out set (17 usages)
~~~

## Results

| Set | Scanner | Confident hits: precision / recall | All hits: precision / recall |
|---|---|---|---|
| Benchmark (24 usages) | v0.1.0 | 100.0% / 79.2% | 82.8% / 100.0% |
| Benchmark (24 usages) | v0.2.0 | 100.0% / 100.0% | 82.8% / 100.0% |
| Held-out (17 usages) | v0.1.0 | 100.0% / 76.5% | 85.0% / 100.0% |
| Held-out (17 usages) | v0.2.0 | 100.0% / 88.2% | 85.0% / 100.0% |

What changed in v0.2.0: URLs held in constants, object members and small helper functions are resolved; untyped data passed into functions is followed up to 3 calls deep; `for...of` loops over response data are followed; wrappers defined as object methods (`api.load()`) or class methods (`svc.load()`) are recognized.

## Read these numbers carefully

- Both sets were written by the author. The benchmark set was used to decide what to fix, so its 100% is by construction, not evidence.
- The held-out set was written before the fixes, but two of its four misses were then fixed too, so 88.2% is only partly independent.
- The sets are small (24 and 17 usages). Treat the numbers as a regression test and a direction, not as a promise for your codebase.
- The extra "all hits" lines are decoys on purpose: unrelated types that happen to have a field with the same name. Hide them with `--no-name-only`.
- Real-world precision and recall are not measured yet.

## Known misses in the held-out set

- A value stored in React state (`useState`, set from the fetch in an effect) and read through a hook.
- A value that travels through a Redux `action.payload` typed as `any`.