export async function loadProfile() {
  const res = await fetch("http://localhost:9000/api/users/7");
  const data = await res.json();
  const { user_name } = data;
  return user_name.toUpperCase();
}