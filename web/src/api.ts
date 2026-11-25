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
  token: string;
  actions: { impact: boolean; fix: boolean; approve: boolean };
};

export type DriftItem = { id: number; time: string; v: Violation };

export async function getState(): Promise<State> {
  const r = await fetch("/api/state");
  if (!r.ok) throw new Error(`state ${r.status}`);
  return r.json();
}
export type ContractNode = {
  kinds: string[];
  properties?: Record<string, ContractProp>;
  element?: ContractNode;
  map_value?: ContractNode;
};
export type ContractProp = { order?: number; required: boolean; node: ContractNode };
export type ContractEndpoint = { sample_count: number; schema: ContractNode };
export type ContractFile = {
  schemaVersion: string;
  generated_at: string;
  tool_version: string;
  approved_by?: string;
  approved_at?: string;
  endpoints: Record<string, ContractEndpoint>;
};

// null means there is no approved contract yet.
export async function getContract(): Promise<ContractFile | null> {
  const r = await fetch("/api/contract");
  if (r.status === 503 || r.status === 500) return null;
  if (!r.ok) throw new Error(`contract ${r.status}`);
  return r.json();
}

export type ImpactHit = { file: string; line: number; kind: string; confidence: string; snippet: string };
export type Impact = { endpoint: string; path: string; endpointCalls: number; hits: ImpactHit[]; files: number };
export type FixChange = { file: string; line: number; confidence: string; before: string; after: string };
export type FixResult = {
  edits: number;
  files: number;
  changes: FixChange[];
  applied: boolean;
  reverted: boolean;
  diagnostics?: { before: number; after: number };
};

export const msgOf = (e: unknown): string => (e instanceof Error ? e.message : String(e));

async function parse<T>(r: Response): Promise<T> {
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((body as { error?: string }).error ?? `request failed (${r.status})`);
  return body as T;
}

const query = (o: Record<string, string>) => new URLSearchParams(o).toString();

export async function getImpact(endpoint: string, path: string): Promise<Impact> {
  return parse<Impact>(await fetch(`/api/impact?${query({ endpoint, path })}`));
}

export async function getFixPreview(endpoint: string, path: string, to: string): Promise<FixResult> {
  return parse<FixResult>(await fetch(`/api/fix/preview?${query({ endpoint, path, to })}`));
}

// Actions that change files need the per-run token from /api/state.
export async function post<T>(url: string, token: string, body: unknown): Promise<T> {
  return parse<T>(
    await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-DevLatch-Token": token },
      body: JSON.stringify(body),
    }),
  );
}
