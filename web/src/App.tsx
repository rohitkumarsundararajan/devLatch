import { useEffect, useMemo, useState } from "react";
import { DriftItem, State, Violation, getState, msgOf, post } from "./api";
import Contract from "./Contract";
import Detail from "./Detail";

const MAX_FEED = 200;

function describe(v: Violation): string {
  if (v.rename_from && v.rename_to) {
    const c = v.confidence ? ` (${Math.round(v.confidence * 100)}%)` : "";
    return `${v.rename_from} \u2192 ${v.rename_to}${c}`;
  }
  if (v.expected?.length || v.actual?.length) {
    return `${(v.expected ?? []).join("|") || "-"} \u2192 ${(v.actual ?? []).join("|") || "-"}`;
  }
  return "";
}

export default function App() {
  const [state, setState] = useState<State | null>(null);
  const [live, setLive] = useState(false);
  const [feed, setFeed] = useState<DriftItem[]>([]);
  const [tab, setTab] = useState<"live" | "contract">("live");
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    let stop = false;
    const refresh = () =>
      getState()
        .then((s) => !stop && setState(s))
        .catch(() => {});
    refresh();
    const timer = setInterval(refresh, 2000);

    const es = new EventSource("/api/events");
    es.onopen = () => setLive(true);
    es.onerror = () => setLive(false);
    es.addEventListener("drift", (e) => {
      const me = e as MessageEvent;
      const id = Number(me.lastEventId);
      const item: DriftItem = { id, time: new Date().toISOString(), v: JSON.parse(me.data) };
      setFeed((f) => (f.some((x) => x.id === id) ? f : [item, ...f].slice(0, MAX_FEED)));
    });
    es.addEventListener("endpoint", refresh);
    es.addEventListener("action", (e) => {
      const a = JSON.parse((e as MessageEvent).data) as { kind: string; endpoint: string };
      if (a.kind === "approve") {
        setFeed((f) => (a.endpoint ? f.filter((x) => x.v.endpoint !== a.endpoint) : []));
      }
    });

    return () => {
      stop = true;
      clearInterval(timer);
      es.close();
    };
  }, []);

  const drifted = useMemo(() => new Set(feed.map((d) => `${d.v.endpoint}|${d.v.path}`)), [feed]);
  const selected = feed.find((d) => d.id === selectedId) ?? null;

  async function approveAll() {
    if (!state) return;
    const ok = window.confirm(
      `Accept the current live shape of ${state.endpoints.length} endpoint(s) as the approved contract? ` +
        "Endpoints without live traffic keep their existing contract.",
    );
    if (!ok) return;
    try {
      await post("/api/approve", state.token, { endpoint: "" });
      setFeed([]);
      setNotice("Approved the current live traffic as the contract.");
    } catch (e) {
      setNotice(`Approve failed: ${msgOf(e)}`);
    }
  }

  return (
    <div className="app">
      <header>
        <h1>DevLatch</h1>
        <span className={live ? "pill ok" : "pill off"}>{live ? "live" : "disconnected"}</span>
        {state && <span className="muted">v{state.version}</span>}
        <nav className="tabs">
          <button className={tab === "live" ? "on" : ""} onClick={() => setTab("live")}>
            Live
          </button>
          <button className={tab === "contract" ? "on" : ""} onClick={() => setTab("contract")}>
            Contract
          </button>
        </nav>
      </header>

      {notice && (
        <div className="banner" onClick={() => setNotice("")}>
          {notice} <span className="muted">(click to dismiss)</span>
        </div>
      )}

      {tab === "contract" ? (
        <Contract drifted={drifted} />
      ) : (
        <>
          <main>
            <section>
              <div className="detail-head">
                <h2>Endpoints ({state?.endpoints.length ?? 0})</h2>
                {state?.actions.approve && state.endpoints.length > 0 && (
                  <button className="btn" onClick={approveAll}>
                    Approve all live traffic
                  </button>
                )}
              </div>
              {state && state.endpoints.length === 0 && (
                <p className="muted">No traffic yet. Point your app at the DevLatch proxy.</p>
              )}
              <ul className="list">
                {state?.endpoints.map((e) => (
                  <li key={e.key}>
                    <code>{e.key}</code>
                    <span className="muted">{e.responses} responses</span>
                  </li>
                ))}
              </ul>
            </section>

            <section>
              <h2>Drift feed ({feed.length})</h2>
              {feed.length === 0 && <p className="muted">No drift detected.</p>}
              <ul className="list">
                {feed.map((d) => (
                  <li
                    key={d.id}
                    className={d.id === selectedId ? "click sel" : "click"}
                    onClick={() => setSelectedId(d.id)}
                  >
                    <span className={`sev ${d.v.severity}`}>{d.v.severity}</span>
                    <div>
                      <div>
                        <strong>{d.v.type}</strong> <code>{d.v.path}</code>
                      </div>
                      <div className="muted">
                        <code>{d.v.endpoint}</code> {describe(d.v)}
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          </main>
          {selected && state && (
            <Detail
              item={selected}
              state={state}
              onClose={() => setSelectedId(null)}
              onApproved={(ep) => {
                setFeed((f) => f.filter((x) => x.v.endpoint !== ep));
                setSelectedId(null);
                setNotice(`Approved ${ep} as the new contract.`);
              }}
            />
          )}
        </>
      )}
    </div>
  );
}