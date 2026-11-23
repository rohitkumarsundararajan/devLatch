import type { OrdersResponse, User } from "./types";

export async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  return res.json();
}

export const getUser = (id: number) => getJSON<User>(`/api/users/${id}`);

export const getOrders = () => getJSON<OrdersResponse>("/api/orders");

export async function loadUserRaw(id: number) {
  const r = await fetch(`/api/users/${id}`);
  return r.json();
}