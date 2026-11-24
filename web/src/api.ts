export type Violation = {
  endpoint: string;
  path: string;
  type: string;
  severity: "BREAKING" | "WARNING" | "SAFE";
  expected?: string[];
  actual?: string[];
  rename_from?: string;
  rename_to?: string;
  confidence?: number;
  evidence?: string[];
};

export type EndpointInfo = { key: string; responses: number };

export type State = {
  version: string;
  endpoints: EndpointInfo[];
  last_event_id: number;
  drift_events: number;
};

export type DriftItem = { id: number; time: string; v: Violation };

export async function getState(): Promise<State> {
  const r = await fetch("/api/state");
  if (!r.ok) throw new Error(`state ${r.status}`);
  return r.json();
}