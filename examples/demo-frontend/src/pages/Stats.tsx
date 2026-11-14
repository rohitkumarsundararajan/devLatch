import { useEffect, useState } from "react";
import type { DailyStatsResponse } from "../types/stats";

// Direct fetch (no client wrapper) on purpose: gives the Phase 4/8 scanner a
// second call style to link to its endpoint.
export default function Stats() {
  const [data, setData] = useState<DailyStatsResponse | null>(null);

  useEffect(() => {
    fetch("/api/v1/stats/daily")
      .then((res) => res.json() as Promise<DailyStatsResponse>)
      .then(setData)
      .catch(() => setData(null));
  }, []);

  if (!data) return <p>Loading...</p>;

  return (
    <table>
      <thead>
        <tr>
          <th>Day</th>
          <th>Orders</th>
          <th>Revenue</th>
        </tr>
      </thead>
      <tbody>
        {Object.entries(data.by_day).map(([day, s]) => (
          <tr key={day}>
            <td>{day}</td>
            <td>{s.orders}</td>
            <td>{s.revenue.toFixed(1)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
