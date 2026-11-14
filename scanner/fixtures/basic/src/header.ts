import { getUser } from "./api";
import { User } from "./types";
export function renderHeader(user: User) {
  return `Hello ${user.user_name}`;
}
export async function loadHeader() {
  const u = await getUser(1);
  return u.email;
}