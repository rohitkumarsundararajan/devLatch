import type { Invoice, Product } from "./types";

export const productOwner = (x: Product) => x.user_name;
export const invoiceTotal = (i: Invoice) => i.total_amount;
export const loose = (o: { user_name: string }) => o.user_name;