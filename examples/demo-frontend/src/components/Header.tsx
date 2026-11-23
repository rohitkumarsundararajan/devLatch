import type { User } from "../types/user";

export default function Header({ user }: { user: User | null }) {
  return (
    <header>
      <strong>Demo Shop</strong>
      {user && <span> | signed in as {user.username}</span>}
    </header>
  );
}
