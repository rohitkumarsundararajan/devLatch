import type { User } from "../types/user";

export default function Profile({ user }: { user: User }) {
  const { username: user_name, email, role_id, nickname, permissions, phone } = user;
  return (
    <section>
      <h2>{user_name}</h2>
      <p>Email: {email}</p>
      <p>Role: {role_id}</p>
      <p>Nickname: {nickname.toUpperCase()}</p>
      <p>Permissions: {permissions.join(", ")}</p>
      {phone && <p>Phone: {phone}</p>}
    </section>
  );
}
