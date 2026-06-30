import { http } from './http'
import { siteApi } from './site'

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
    http.get<{ games: Game[] }>(`/entertainment/${siteId}/games`).then((r) => r.data.games),
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
      .then((r) => r.data.options),
  /** 购卡 */
  buy: (siteId: number, game: string, optionType: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/buy`, { option_type: optionType })
      .then((r) => r.data),
  /** 奖品展示 */
  prizes: (siteId: number, game: string) =>
    http
      .get<{ prizes: PrizeDef[] }>(`/entertainment/${siteId}/games/${game}/prizes`)
      .then((r) => r.data.prizes),
  /** 抽奖记录 */
  history: (siteId: number, game: string) =>
    http
      .get<{ records: GameRecord[] }>(`/entertainment/${siteId}/games/${game}/history`)
      .then((r) => r.data.records),
  /** 卡牌合成目标列表（含碎片库存） */
  syntheses: (siteId: number, game: string) =>
    http
      .get<{ targets: SynthesisTarget[] }>(`/entertainment/${siteId}/games/${game}/syntheses`)
      .then((r) => r.data.targets),
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
    http.get<{ nodes: MedalNode[] }>(`/entertainment/${siteId}/games/${game}/medal-tree`).then((r) => r.data.nodes),
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
  /** 二十一点动作（new/hit/stand/double） */
  blackjack: (siteId: number, game: string, action: string) =>
    http.post<BlackjackResult>(`/entertainment/${siteId}/games/${game}/blackjack`, { action }).then((r) => r.data),
  /** 站点列表（复用 siteApi） */
  sites: siteApi.list,
}
