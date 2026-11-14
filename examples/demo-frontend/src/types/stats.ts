export interface DayStats {
  orders: number;
  revenue: number;
}

export interface DailyStatsResponse {
  by_day: Record<string, DayStats>;
}
