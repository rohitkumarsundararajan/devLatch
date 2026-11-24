import { useEffect, useState } from "react";
import { DriftItem, State, Violation, getState } from "./api";

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

    return () => {
      stop = true;
      clearInterval(timer);
      es.close();
    };
  }, []);

  return (
    <div className="app">
      <header>
        <h1>DevLatch</h1>
        <span className={live ? "pill ok" : "pill off"}>{live ? "live" : "disconnected"}</span>
        {state && <span className="muted">v{state.version}</span>}
      </header>

      <main>
        <section>
          <h2>Endpoints ({state?.endpoints.length ?? 0})</h2>
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
              <li key={d.id}>
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
    </div>
  );
}