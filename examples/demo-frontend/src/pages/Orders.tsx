import { useEffect, useState } from "react";
import { getOrders } from "../api/client";
import type { Order } from "../types/order";

export default function Orders() {
  const [orders, setOrders] = useState<Order[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getOrders()
      .then((res) => setOrders(res.orders))
      .catch((e: unknown) => setError(String(e)));
  }, []);

  if (error) return <p className="error">{error}</p>;

  return (
    <table>
      <thead>
        <tr>
          <th>Order</th>
          <th>Balance</th>
          <th>Items</th>
          <th>Tags</th>
        </tr>
      </thead>
      <tbody>
        {orders.map((order) => (
          <tr key={order.id}>
            <td>{order.id}</td>
            {/* Throws if balance becomes a string: the classic silent-in-CI break. */}
            <td>{order.balance.toFixed(2)}</td>
            <td>{order.items}</td>
            <td>
              {order.tags.map((tag) => (
                <span className="tag" key={tag}>
                  {tag}
                </span>
              ))}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
