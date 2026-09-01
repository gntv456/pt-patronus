// 蜂巢五子棋类型定义（自 entertainmentTypes.ts 拆出，行数门禁）。

export interface GobangMove {
  x: number // 列 0..14
  y: number // 行 0..14
  step?: number // 手数（1 起）
  player_id?: number // 0=AI；否则用户 id
}
export interface GobangMatch {
  id: string
  status: string // ONGOING | FINISHED
  difficulty: string // WARMUP | STANDARD | HARD | NIGHTMARE
  board: number[][] // 15x15：0 空 / 1 黑 / 2 白
  moves?: GobangMove[]
  player_color?: string // BLACK | WHITE
  current_side?: string // PLAYER | AI；结束局为空
  first_hand?: string // PLAYER | AI
  rule?: string // FREESTYLE（无禁手）
  opening_choice_pending?: boolean
  opening_choice_deadline_at?: string
  opening_move_count?: number
  ticket_charged?: boolean
  ticket_cost?: number
  win_reward?: number
  undo_cost?: number
  undo_used?: boolean
  ranked?: boolean
  winner_id?: number | null // 0=AI 胜；null=平局；缺省=进行中
  finish_reason?: string
  payout_amount?: number
  interruption_voided_at?: string
  move_deadline_at?: string
  total_deadline_at?: string
  credit_settlement_enabled?: boolean
  finished_at?: string
  updated_at?: string
}
export interface GobangSummary {
  points: number
  point_name: string
  legacy_point_name?: string
  total_limit: number
  total_used: number
  total_remaining: number
  warmup_remaining: number
  nightmare_remaining?: number
  free_total?: number
  free_used?: number
  free_remaining?: number
  paid_total?: number
  paid_used?: number
  paid_remaining?: number
  daily_net_gain: number
  daily_net_gain_cap: number
}
export interface GobangRankEntry {
  user_id: number
  rank: number
  username: string
  nickname: string
  avatar_path?: string
  games: number
  wins: number
  losses: number
  win_rate: number
  average_duration_seconds: number
  average_moves: number
  player_first_games: number
  ai_first_games: number
  score: number
}
export interface GobangState {
  matches: GobangMatch[]
  summary?: GobangSummary
  leaderboards?: Record<string, GobangRankEntry[]>
}
