export interface Product { user_name: string }
export function label(p: Product) {
  return p.user_name;
}