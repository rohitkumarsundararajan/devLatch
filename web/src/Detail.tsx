import { useEffect, useState } from "react";
import { DriftItem, FixResult, Impact, State, getFixPreview, getImpact, msgOf, post } from "./api";

const IDENT = /^[A-Za-z_$][A-Za-z0-9_$]*$/;

type Props = {
  item: DriftItem;
  state: State;
  onClose: () => void;
  onApproved: (endpoint: string) => void;
};

export default function Detail({ item, state, onClose, onApproved }: Props) {
  const v = item.v;
  const to = v.rename_to ?? "";
  const canFix = state.actions.fix && IDENT.test(to);

  const [impact, setImpact] = useState<Impact | null>(null);
  const [impactErr, setImpactErr] = useState("");
  const [preview, setPreview] = useState<FixResult | null>(null);
  const [previewErr, setPreviewErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const [err, setErr] = useState("");

  const loadImpact = () =>
    getImpact(v.endpoint, v.path)
      .then((r) => {
        setImpact(r);
        setImpactErr("");
      })
      .catch((e) => setImpactErr(msgOf(e)));

  useEffect(() => {
    let stop = false;
    setImpact(null);
    setImpactErr("");
    setPreview(null);
    setPreviewErr("");
    setMsg("");
    setErr("");
    if (state.actions.impact) {
      getImpact(v.endpoint, v.path)
        .then((r) => !stop && setImpact(r))
        .catch((e) => !stop && setImpactErr(msgOf(e)));
    }
    if (canFix) {
      getFixPreview(v.endpoint, v.path, to)
        .then((r) => !stop && setPreview(r))
        .catch((e) => !stop && setPreviewErr(msgOf(e)));
    }
    return () => {
      stop = true;
    };
  }, [item.id]);

  async function applyFix() {
    if (!preview) return;
    const ok = window.confirm(
      `Rewrite ${preview.edits} usage(s) in ${preview.files} file(s) of your frontend? This changes files on disk.`,
    );
    if (!ok) return;
    setBusy(true);
    setErr("");
    try {
      const r = await post<FixResult>("/api/fix/apply", state.token, { endpoint: v.endpoint, path: v.path, to });
      const ts = r.diagnostics ? ` TypeScript errors: ${r.diagnostics.before} -> ${r.diagnostics.after}.` : "";
      setMsg(`Applied ${r.edits} edit(s) in ${r.files} file(s).${ts}`);
      setPreview(null);
      void loadImpact();
    } catch (e) {
      setErr(msgOf(e));
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    const ok = window.confirm(`Accept the current live shape of ${v.endpoint} as the approved contract?`);
    if (!ok) return;
    setBusy(true);
    setErr("");
    try {
      await post("/api/approve", state.token, { endpoint: v.endpoint });
      onApproved(v.endpoint);
    } catch (e) {
      setErr(msgOf(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="detail">
      <div className="detail-head">
        <h2>
          <span className={`sev ${v.severity}`}>{v.severity}</span> {v.type} <code>{v.path}</code>
        </h2>
        <button className="btn" onClick={onClose}>
          Close
        </button>
      </div>
      <p className="muted">
        <code>{v.endpoint}</code>
        {v.rename_from && v.rename_to ? (
          <>
            {" "}
            &middot; likely rename <code>{v.rename_from}</code> &rarr; <code>{v.rename_to}</code>
          </>
        ) : null}
      </p>
      {v.evidence && v.evidence.length > 0 && <p className="muted">{v.evidence.join("; ")}</p>}

      <h3>Affected frontend code</h3>
      {!state.actions.impact && (
        <p className="muted">Start devlatch ui with --frontend &lt;folder&gt; to see affected files and fixes.</p>
      )}
      {impactErr && <p className="error">{impactErr}</p>}
      {state.actions.impact && !impact && !impactErr && <p className="muted">Scanning...</p>}
      {impact && (
        <>
          <p className="muted">
            {impact.hits.length} usage(s) in {impact.files} file(s)
          </p>
          <ul className="list">
            {impact.hits.map((h, i) => (
              <li key={i}>
                <div>
                  <code>
                    {h.file}:{h.line}
                  </code>{" "}
                  <span className="muted">
                    {h.kind}, {h.confidence}
                  </span>
                  <pre>{h.snippet}</pre>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}

      {state.actions.fix && v.rename_to && (
        <>
          <h3>Fix preview</h3>
          {!canFix && <p className="muted">&quot;{to}&quot; is not a valid field name, so no automatic fix.</p>}
          {previewErr && <p className="error">{previewErr}</p>}
          {canFix && !preview && !previewErr && !msg && <p className="muted">Preparing preview...</p>}
          {preview && preview.edits === 0 && <p className="muted">Nothing to change: no usages of the old field were found.</p>}
          {preview && preview.edits > 0 && (
            <>
              <p className="muted">
                {preview.edits} edit(s) in {preview.files} file(s). Nothing is written until you press Apply.
              </p>
              <ul className="list">
                {preview.changes.map((c, i) => (
                  <li key={i}>
                    <div>
                      <code>
                        {c.file}:{c.line}
                      </code>{" "}
                      <span className="muted">{c.confidence}</span>
                      <pre className="del">- {c.before}</pre>
                      <pre className="add">+ {c.after}</pre>
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}

      {msg && <p className="ok-text">{msg}</p>}
      {err && <p className="error">{err}</p>}
      <div className="actions">
        {preview && preview.edits > 0 && (
          <button className="btn primary" disabled={busy} onClick={applyFix}>
            Apply fix to frontend
          </button>
        )}
        {state.actions.approve && (
          <button className="btn" disabled={busy} onClick={approve}>
            Approve this endpoint as the new contract
          </button>
        )}
      </div>
    </section>
  );
}