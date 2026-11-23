export interface User {
  id: number;
  user_name: string; // @hit u1
  email: string;
}

export interface Product {
  id: number;
  user_name: string;
}

export type UserCard = Pick<User, "user_name" | "id">;

export interface Order {
  id: number;
  total_amount: number; // @hit o1
}

export interface OrdersResponse {
  orders: Order[];
}

export interface Invoice {
  id: number;
  total_amount: number;
}