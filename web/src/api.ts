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
