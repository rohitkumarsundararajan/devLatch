export interface User {
  id: string;
  user_name: string;
  email: string;
  role_id: number;
  nickname: string;
  permissions: string[];
  phone?: string;
}
