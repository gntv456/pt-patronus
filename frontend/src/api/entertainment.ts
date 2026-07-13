import { http } from './http'
import type { Site } from './site'

// 复用 site 类型，避免重复定义
export type { Site }

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

export const entertainmentApi = {
  /** 某站点可玩的游戏 */
  games: (siteId: number) =>
    http.get<{ games: Game[] }>(`/entertainment/${siteId}/games`).then((r) => r.data.games ?? []),
  /** 玩一次 */
  play: (siteId: number, game: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/play`, { game }).then((r) => r.data),
  /** 连抽（仅 LongPT 大转盘等支持） */
  playMulti: (siteId: number, game: string, count: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/play-multi`, { count })
      .then((r) => r.data),
  /** 游戏统计（余额/剩余刮卡） */
  stats: (siteId: number, game: string) =>
    http.get<GameStats>(`/entertainment/${siteId}/games/${game}/stats`).then((r) => r.data),
  /** 购卡档位 */
  buyOptions: (siteId: number, game: string) =>
    http
      .get<{ options: BuyOption[] }>(`/entertainment/${siteId}/games/${game}/buy-options`)
      .then((r) => r.data.options ?? []),
  /** 购卡 */
  buy: (siteId: number, game: string, optionType: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/buy`, { option_type: optionType })
      .then((r) => r.data),
  /** 奖品展示 */
  prizes: (siteId: number, game: string) =>
    http
      .get<{ prizes: PrizeDef[] }>(`/entertainment/${siteId}/games/${game}/prizes`)
      .then((r) => r.data.prizes ?? []),
  /** 抽奖记录 */
  history: (siteId: number, game: string) =>
    http
      .get<{ records: GameRecord[] }>(`/entertainment/${siteId}/games/${game}/history`)
      .then((r) => r.data.records ?? []),
  /** 卡牌合成目标列表（含碎片库存） */
  syntheses: (siteId: number, game: string) =>
    http
      .get<{ targets: SynthesisTarget[] }>(`/entertainment/${siteId}/games/${game}/syntheses`)
      .then((r) => r.data.targets ?? []),
  /** 抽碎片（1/10/100/其它） */
  draw: (siteId: number, game: string, synthesisId: number, count: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/draw`, {
        synthesis_id: synthesisId,
        count,
      })
      .then((r) => r.data),
  /** 合成 */
  synthesize: (siteId: number, game: string, synthesisId: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/synthesize`, {
        synthesis_id: synthesisId,
      })
      .then((r) => r.data),
  /** 勋章金字塔（六十四卦） */
  medalTree: (siteId: number, game: string) =>
    http.get<{ nodes: MedalNode[] }>(`/entertainment/${siteId}/games/${game}/medal-tree`).then((r) => r.data.nodes ?? []),
  /** 阳爻/阴爻碎片余额 */
  medalShards: (siteId: number, game: string) =>
    http.get<{ shards: MedalShard }>(`/entertainment/${siteId}/games/${game}/medal-shards`).then((r) => r.data.shards),
  /** 碎片合成（太极/两仪/四象） */
  composeMedal: (siteId: number, game: string, medalId: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/compose`, { medal_id: medalId }).then((r) => r.data),
  /** 大勋章合成（八卦/六十四卦，耗魔力+子勋章） */
  composeBigMedal: (siteId: number, game: string, medalId: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/compose-big`, { medal_id: medalId }).then((r) => r.data),
  /** 阴阳转换（yi_yang>0 阴→阳，<0 阳→阴） */
  convertShard: (siteId: number, game: string, yiYang: number) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/convert-shard`, { yi_yang: yiYang }).then((r) => r.data),
  /** 农场全景（NovaHD 魔力农场） */
  farm: (siteId: number, game: string) =>
    http.get<FarmState>(`/entertainment/${siteId}/games/${game}/farm`).then((r) => r.data),
  /** 种植/养殖（section=crop→plant / animal→breed） */
  plant: (siteId: number, game: string, section: string, itemId: number) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/plant`, { section, item_id: itemId }).then((r) => r.data),
  /** 收获单个 */
  harvest: (siteId: number, game: string, section: string, itemId: number) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/harvest`, { section, item_id: itemId }).then((r) => r.data),
  /** 一键收获 */
  harvestAll: (siteId: number, game: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/harvest-all`, {}).then((r) => r.data),
  /** 出售仓库物品 */
  sell: (siteId: number, game: string, key: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/sell`, { key }).then((r) => r.data),
  /** 下注/选号（猜大小/骰宝/双色球/翻面/Betting/红包） */
  bet: (siteId: number, game: string, req: BetRequest) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bet`, req).then((r) => r.data),
  /** 赛事押注：列竞猜主题 */
  events: (siteId: number, game: string) =>
    http.get<{ events: BetEvent[] }>(`/entertainment/${siteId}/games/${game}/events`).then((r) => r.data.events ?? []),
  /** 赛事押注：某主题盘口（选项+赔率） */
  eventDetail: (siteId: number, game: string, eventId: number) =>
    http.get<BetEventDetail>(`/entertainment/${siteId}/games/${game}/events/${eventId}`).then((r) => r.data),
  /** 翻面夺宝全景（PandaPT：当前轮次/余额/100 格状态/奖项/记录） */
  flipState: (siteId: number, game: string) =>
    http.get<FlipState>(`/entertainment/${siteId}/games/${game}/flip-state`).then((r) => r.data),
  /** 猜大小全景快照（走势/排行/记录/当前局/任务/道具/聊天/红包） */
  bigsmallState: (siteId: number, game: string) =>
    http.get<BigSmallState>(`/entertainment/${siteId}/games/${game}/bigsmall-state`).then((r) => r.data),
  /** 猜大小拉新聊天（since：好学 last_id；熊猫忽略） */
  bigsmallChat: (siteId: number, game: string, since: number) =>
    http
      .get<{ chat: BigSmallChat[] }>(`/entertainment/${siteId}/games/${game}/bigsmall-chat`, { params: { since } })
      .then((r) => r.data.chat ?? []),
  /** 猜大小发聊天 */
  bigsmallSendChat: (siteId: number, game: string, message: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-chat`, { message }).then((r) => r.data),
  /** 猜大小任务（claim/reward/refund） */
  bigsmallTask: (siteId: number, game: string, action: string, taskId: number) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-task`, { action, task_id: taskId }).then((r) => r.data),
  /** 猜大小使用道具 */
  bigsmallItem: (siteId: number, game: string, tag: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-item`, { tag }).then((r) => r.data),
  /** 猜大小红包（send/claim/info） */
  bigsmallRedpacket: (siteId: number, game: string, action: string, extra: Record<string, string>) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-redpacket`, { action, extra }).then((r) => r.data),
  /** 二十一点动作（new/hit/stand/double） */
  blackjack: (siteId: number, game: string, action: string) =>
    http.post<BlackjackResult>(`/entertainment/${siteId}/games/${game}/blackjack`, { action }).then((r) => r.data),
  /** 站点列表（含各站已适配娱乐项目数 game_count，独立于通用 /sites） */
  sites: () => http.get<{ sites: Site[] }>('/entertainment').then((r) => r.data.sites ?? []),
}
