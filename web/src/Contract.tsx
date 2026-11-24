import { useEffect, useState } from "react";
import { ContractFile, ContractNode, getContract } from "./api";

type Drifted = Set<string>;

function kindsText(n?: ContractNode): string {
  return (n?.kinds ?? []).join(" | ");
}

function NodeRows(props: { node: ContractNode; path: string; endpoint: string; drifted: Drifted }) {
  const { node, path, endpoint, drifted } = props;
  const fields = Object.entries(node.properties ?? {}).sort(
    ([an, a], [bn, b]) => (a.order || 1e9) - (b.order || 1e9) || an.localeCompare(bn),
  );
  if (fields.length === 0 && !node.element && !node.map_value) return null;
  return (
    <ul className="tree">
      {fields.map(([name, p]) => {
        const pp = `${path}.${name}`;
        const hit = drifted.has(`${endpoint}|${pp}`);
        return (
          <li key={name}>
            <div className={hit ? "row hit" : "row"}>
              <code>
                {name}
                {p.required ? "" : "?"}
              </code>
              <span className="muted">{kindsText(p.node)}</span>
              {hit && <span className="sev BREAKING">DRIFT</span>}
            </div>
            {p.node && <NodeRows node={p.node} path={pp} endpoint={endpoint} drifted={drifted} />}
          </li>
        );
      })}
      {node.element && (
        <li>
          <div className="row">
            <code>[ ]</code>
            <span className="muted">{kindsText(node.element)}</span>
          </div>
          <NodeRows node={node.element} path={`${path}[]`} endpoint={endpoint} drifted={drifted} />
        </li>
      )}
      {node.map_value && (
        <li>
          <div className="row">
            <code>{"{ * }"}</code>
            <span className="muted">{kindsText(node.map_value)}</span>
          </div>
          <NodeRows node={node.map_value} path={`${path}.*`} endpoint={endpoint} drifted={drifted} />
        </li>
      )}
    </ul>
  );
}

export default function Contract({ drifted }: { drifted: Drifted }) {
  const [file, setFile] = useState<ContractFile | null | undefined>(undefined);
  const [err, setErr] = useState("");

  useEffect(() => {
    getContract()
      .then(setFile)
      .catch((e) => setErr(String(e)));
  }, []);

  if (err) return <p className="muted">Could not load the contract: {err}</p>;
  if (file === undefined) return <p className="muted">Loading contract...</p>;
  if (file === null) {
    return <p className="muted">No approved contract yet. Run devlatch record, then devlatch approve.</p>;
  }

  const keys = Object.keys(file.endpoints).sort();
  return (
    <section>
      <h2>Approved contract ({keys.length} endpoints)</h2>
      <p className="muted">
        schema {file.schemaVersion} &middot; tool {file.tool_version} &middot; generated {file.generated_at}
        {file.approved_at ? ` \u00b7 approved ${file.approved_at}` : ""}
        {file.approved_by ? ` by ${file.approved_by}` : ""}
      </p>
      {keys.map((k) => {
        const ep = file.endpoints[k];
        return (
          <details key={k}>
            <summary>
              <code>{k}</code> <span className="muted">{ep.sample_count} samples</span>
            </summary>
            <div className="row">
              <code>$</code>
              <span className="muted">{kindsText(ep.schema)}</span>
            </div>
            <NodeRows node={ep.schema} path="$" endpoint={k} drifted={drifted} />
          </details>
        );
      })}
    </section>
  );
}