import { api } from "./client";
import type { Profile, Stats } from "./types";

declare function useState<T>(): [T | undefined, (v: T) => void];
declare function useEffect(f: () => void): void;

export async function typed() { const s = await api.stats(); return s.visit_count; } // @hit s1
export async function both() { const [s, p] = await Promise.all([api.stats(), api.profile()]); return [s.visit_count, p.avatar_url]; } // @hit s1 // @hit p1
export const total = (rows: Stats[]) => rows.reduce((a, r) => a + r.visit_count, 0); // @hit s1
export const copy = (s: Stats) => { const c = { ...s }; return c.visit_count; }; // @hit s1
export const withDefault = (s: Stats) => { const { visit_count: v = 0 } = s; return v; }; // @hit s1
export const fallback = (s: Stats | null, d: Stats) => (s ?? d).visit_count; // @hit s1
export function Chart({ stats }: { stats: Stats }) { return stats.visit_count; } // @hit s1
export function Avatar(p: Profile) { return p.avatar_url; } // @hit p1

export async function fromText() { const t = await (await fetch("/api/stats")).text(); return JSON.parse(t).visit_count; } // @hit s1
export async function rawViaProp() { const s = await api.statsRaw(); return s.visit_count; } // @hit s1

export class Svc {
  load() { return fetch("/api/stats").then((r) => r.json()); }
}
export async function viaClass(svc: Svc) { const s = await svc.load(); return s.visit_count; } // @hit s1

export function useStats() {
  const [d, setD] = useState<any>();
  useEffect(() => { fetch("/api/stats").then((r) => r.json()).then(setD); });
  return d;
}
export function Widget() { const stats = useStats(); return stats?.visit_count; } // @hit s1

export function reducer(state: any, action: { type: string; payload: any }) { return action.payload.visit_count; } // @hit s1

export const reportCount = (r: { visit_count: number }) => r.visit_count;