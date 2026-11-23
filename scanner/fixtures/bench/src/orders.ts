import { getOrders } from "./api";
import type { Order, OrdersResponse } from "./types";

export async function sum() { const r = await getOrders(); return r.orders.reduce((a, o) => a + o.total_amount, 0); } // @hit o1
export function fmt(o: Order) { return `${o.total_amount}`; } // @hit o1
export async function raw() { const j = await (await fetch("/api/orders")).json(); return j.orders.map((o: any) => o.total_amount); } // @hit o1
export function viaFor(r: OrdersResponse) { for (const o of r.orders) { console.log(o.total_amount); } } // @hit o1

export async function rawLoop() { const j = await (await fetch("/api/orders")).json(); for (const o of j.orders) { console.log(o.total_amount); } } // @hit o1