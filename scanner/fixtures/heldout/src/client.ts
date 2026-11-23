import type { Profile, Stats } from "./types";

export async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, init);
  if (!r.ok) throw new Error(String(r.status));
  return (await r.json()) as T;
}

export const api = {
  stats: () => request<Stats>("/api/stats"),
  profile: () => request<Profile>("/api/profile"),
  statsRaw: () => fetch("/api/stats").then((r) => r.json()),
};

declare const axios: { get<T>(u: string): Promise<{ data: T }> };
export const viaAxios = async () => { const res = await axios.get<Stats>("/api/stats"); return res.data.visit_count; }; // @hit s1