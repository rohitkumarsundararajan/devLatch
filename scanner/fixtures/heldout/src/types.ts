export interface Stats {
  visit_count: number; // @hit s1
  region: string;
}

export interface Profile {
  avatar_url: string; // @hit p1
  display_name: string;
}

export interface Report {
  visit_count: number;
  avatar_url: string;
}