import { useEffect, useState } from "react";
import { getUser } from "./api/client";
import Header from "./components/Header";
import Orders from "./pages/Orders";
import Profile from "./pages/Profile";
import Stats from "./pages/Stats";
import type { User } from "./types/user";

type View = "profile" | "orders" | "stats";
const VIEWS: View[] = ["profile", "orders", "stats"];

export default function App() {
  const [view, setView] = useState<View>("profile");
  const [user, setUser] = useState<User | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getUser("7")
      .then(setUser)
      .catch((e: unknown) => setError(String(e)));
  }, []);

  return (
    <>
      <Header user={user} />
      <nav>
        {VIEWS.map((v) => (
          <button key={v} aria-current={v === view ? "page" : undefined} onClick={() => setView(v)}>
            {v}
          </button>
        ))}
      </nav>
      <main>
        {error && <p className="error">{error}</p>}
        {view === "profile" && (user ? <Profile user={user} /> : !error && <p>Loading...</p>)}
        {view === "orders" && <Orders />}
        {view === "stats" && <Stats />}
      </main>
    </>
  );
}
