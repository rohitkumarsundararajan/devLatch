import { getUser, loadUserRaw } from "./api";
import type { User, UserCard } from "./types";

declare function useQuery<T>(o: { queryFn: () => Promise<T> }): { data?: T };

export function title(u: User) { return u.user_name; } // @hit u1
export const safe = (u?: User) => u?.user_name ?? "anon"; // @hit u1
export const byKey = (u: User) => u["user_name"]; // @hit u1
export function destructured({ user_name }: User) { return user_name; } // @hit u1
export const card = (c: UserCard) => c.user_name; // @hit u1
export async function typedWrapper() { const u = await getUser(1); return u.user_name; } // @hit u1
export async function untypedWrapper() { const u = await loadUserRaw(2); return u.user_name; } // @hit u1
export const names = (us: User[]) => us.map((u) => u.user_name); // @hit u1
export function loop(us: User[]) { for (const u of us) console.log(u.user_name); } // @hit u1
export function viaThen() { return fetch("/api/users/3").then((r) => r.json()).then((d) => d.user_name); } // @hit u1

function show(d: any) { return d.user_name; } // @hit u1
export async function viaParam() { const d = await (await fetch("/api/users/4")).json(); return show(d); }

export function Profile() { const { data } = useQuery({ queryFn: () => getUser(5) }); return data?.user_name; } // @hit u1

const USER_URL = "/api/users/6";
export async function viaConst() { const d = await (await fetch(USER_URL)).json(); return d.user_name; } // @hit u1

export class Store {
  user?: User;
  label() { return this.user?.user_name; } // @hit u1
}

function hopA(d: any) { return hopB(d); }
function hopB(d: any) { return d.user_name; } // @hit u1
export async function viaHops() { const d = await (await fetch("/api/users/7")).json(); return hopA(d); }

const API = { user: (id: number) => `/api/users/${id}` };
export async function viaHelper() { const d = await (await fetch(API.user(9))).json(); return d.user_name; } // @hit u1

declare const http: any;
export const viaHttp = () => http.get("/api/users/8").then((r: any) => r.data.user_name); // @hit u1