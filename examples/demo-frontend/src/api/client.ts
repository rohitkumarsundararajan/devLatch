import type { OrdersResponse } from "../types/order";
import type { User } from "../types/user";

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path);
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText} for ${path}`);
  }
  return (await res.json()) as T;
}

export const getUser = (id: string) => getJSON<User>(`/api/v1/users/${id}`);

export const getOrders = () => getJSON<OrdersResponse>("/api/v1/orders");
