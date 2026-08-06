// 娱乐中心类型定义：抽出为独立文件以收敛 entertainment.ts 体积。
// entertainment.ts 会 re-export 这些类型，保持现有 import 路径不变。

// 复用 site 类型，避免重复定义
export type { Site } from './site'

export interface Game {
  id: string
  name: string
}

export interface PrizeOutcome {
  prize_id: number
  prize_name: string
  prize_type: string
  amount: number
  unit_name?: string
  is_win: boolean
}

export interface PlayResult {
  success: boolean
  message?: string
  prize?: string
  prize_id?: number
  amount?: number
  balance?: number
  prizes?: PrizeOutcome[] // 连抽多次结果
  raw?: unknown
}

export interface GameStats {
  balance: number
  card_number: number
  today_spins: number
  max_daily: number
  cost_per_play: number
  // LongPT 等站点可选字段
  discount?: number
  participants?: number
  start_time?: string
  end_time?: string
  open_time?: string
}

export interface BuyOption {
  type: number
  buy_count: number
  gift_count: number
  unit_price: number
}

// 通用下注请求（selection=big/small/点数/赛事id/tile_index；extra=号码等）
export interface BetRequest {
  selection: string
  amount: number
  extra?: Record<string, string>
}

// OurBits 赛事押注（菠菜）：一个押注选项（队名/赔率）。
export interface BetOption {
  id: string // 选项 id（下单 POST 用；非主题 id）
  name: string
  odds: number
  stake?: number
}
// 一场竞猜主题（bet.php 列表行）。
export interface BetEvent {
  id: number
  title: string
  end_time: string
  remain: string
  bookmaker: string
  bet_count: number
  total_stake: number
  my_stake: string
  status: string // open / ended
}
// 某主题盘口（bet_odds.php?id=N）。
export interface BetEventDetail {
  event: BetEvent
  options: BetOption[]
  my_option?: string
  note?: string
}

// 二十一点状态
export interface BlackjackResult {
  player_cards: string[]
  dealer_cards: string[]
  player_score: number
  dealer_score: number
  stage: string
  result: string
  balance: number
  message: string
}

export interface PrizeDef {
  id: number
  name: string
  icon: string
  color: string
  type: string
}

export interface MedalNode {
  medal_id: string
  parent_id: string
  type: string // taiji/liangyi/sixiang/bagua/liushisi
  name: string
  level: number
  image_url: string
  owned: boolean
  need_yang: number
  need_yin: number
  big_cost: number // 大勋章合成魔力（碎片合成为 0）
}

export interface MedalShard {
  yang_yao: number
  yin_yao: number
}

export interface GameRecord {
  prize_name: string
  prize_type: string
  amount: number
  cost: number
  created_at: string
}

// PandaPT 翻面夺宝
export interface FlipTier {
  id: string
  name: string
  reward: number
  reward_type: string
  count: number
  color?: string
}
export interface FlipTile {
  index: number
  flipped: boolean
  tier_id?: string
  by_user?: string
}
export interface FlipState {
  round_number: number
  balance: number
  cost_per_flip: number
  total_tiles: number
  flipped_count: number
  tiers: FlipTier[]
  tiles: FlipTile[]
  history?: GameRecord[]
}

// 猜大小（好学 hxpt_bigsmall / 熊猫 panda_bigsmall）
export interface BigSmallRound {
  id: number
  game_no: string
  start_time?: string
  end_time?: string
  status: string // betting | sealed | finished
  countdown: number
  seal_countdown: number
  player_count: number
  total_bet: number
}
export interface BigSmallBetSpot { username: string; amount: number }
export interface BigSmallMyBet {
  game_no?: string
  bet_type: string
  amount: number
  result?: string
  net_result?: number
}
export interface BigSmallTrend {
  big_count: number
  small_count: number
  house_count?: number
  big_pct: number
  small_pct: number
}
export interface BigSmallRankEntry { username: string; amount: number; count: number }
export interface BigSmallRank { key: string; title: string; entries: BigSmallRankEntry[] }
export interface BigSmallRecord {
  game_no: string
  start_time?: string
  end_time?: string
  player_count?: number
  total_bet?: number
  result: string
  my_bet_type?: string
  my_bet?: number
  my_result?: number
  bets?: string
}
export interface BigSmallTask {
  id: number
  title: string
  desc?: string
  reward?: string
  progress_text?: string
  progress: number
  status: string // idle | doing | rewardable | done
  monthly_count?: number
  monthly_limit?: number
}
export interface BigSmallProp {
  id: string
  tag: string
  name: string
  desc?: string
  image?: string
  quantity: number
  usable: boolean
}
export interface BigSmallChat { id: number; username: string; message: string; time?: string }
export interface BigSmallRedpacket {
  id: string
  sender?: string
  greeting?: string
  total: number
  count: number
  claimed?: number
  my_claim?: number
}
export interface BigSmallState {
  balance: number
  min_bet: number
  max_bet?: number
  currency?: string
  payout?: string
  has_house: boolean
  round: BigSmallRound
  my_bet?: BigSmallMyBet
  last_result?: BigSmallMyBet
  big_bets?: BigSmallBetSpot[]
  small_bets?: BigSmallBetSpot[]
  trend40: BigSmallTrend
  trend10: BigSmallTrend
  trend_text?: string
  recent_results?: string[]
  rankings?: BigSmallRank[]
  my_records?: BigSmallRecord[]
  all_records?: BigSmallRecord[]
  tasks?: BigSmallTask[]
  props?: BigSmallProp[]
  chat?: BigSmallChat[]
  redpackets?: BigSmallRedpacket[]
}

export interface SynthFragment {
  name: string
  image: string
  current: number
  required: number
  is_rare: boolean
}

export interface SynthesisTarget {
  id: number
  name: string
  image: string
  got: number
  need: number
  fragments: SynthFragment[]
  draw_price: number
  ten_price: number
  hundred_price: number
  can_synthesize: boolean
  pity: string
}

// NovaHD 魔力农场
export interface FarmItem {
  id: number
  name: string
  icon?: string
  section: string // crop | animal
  grow_hours: number
  price: number
  expire_days?: number
  status: string // idle | growing | ready
  remain_time?: string
}

export interface StorageItem {
  key: string // 出售用 ?action=sell&key=
  name: string
  icon?: string
  count: number
  market_price?: number
}

export interface MarketItem {
  id: number
  name: string
  section: string // crop | animal
  market_price: number
}

export interface FarmState {
  balance: number
  farm_name?: string
  crops: FarmItem[]
  animals: FarmItem[]
  warehouse?: StorageItem[]
  market?: MarketItem[]
}
