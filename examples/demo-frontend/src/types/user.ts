export interface User {
  id: string;
  username: string;
  email: string;
  role_id: number;
  nickname: string;
  permissions: string[];
  phone?: string;
}
