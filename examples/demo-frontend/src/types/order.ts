export interface Order {
  id: string;
  balance: number;
  items: number;
  tags: string[];
  currency?: string;
}

export interface OrdersResponse {
  page: number;
  orders: Order[];
}
