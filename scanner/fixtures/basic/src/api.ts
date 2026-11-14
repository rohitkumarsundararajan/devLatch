import { User } from "./types";
const BASE = "http://localhost:9000";
export async function getUser(id: number): Promise<User> {
  const res = await fetch(`${BASE}/api/users/${id}`);
  return res.json();
}