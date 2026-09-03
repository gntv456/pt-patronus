import { http } from './http'
// 类型定义抽出至 entertainmentTypes.ts；此处 re-export 保持现有 import 路径（@/api/entertainment）不变。
export type {
  Site,
  Game,
  PrizeOutcome,
  PlayResult,
  GameStats,
  BuyOption,
  BetRequest,
  BetOption,
  BetEvent,
  BetEventDetail,
  BlackjackResult,
  PrizeDef,
  MedalNode,
  MedalShard,
  GameRecord,
  FlipTier,
  FlipTile,
  FlipState,
  BigSmallRound,
  BigSmallBetSpot,
  BigSmallMyBet,
  BigSmallTrend,
  BigSmallRankEntry,
  BigSmallRank,
  BigSmallRecord,
  BigSmallTask,
  BigSmallProp,
  BigSmallChat,
  BigSmallRedpacket,
  BigSmallState,
  SynthFragment,
  SynthesisTarget,
  FarmItem,
  StorageItem,
  MarketItem,
  FarmState,
  GobangMatch,
  GobangState,
  FriendTradeState,
  FishingState,
  City13WheelState,
  City13DiceState,
} from './entertainmentTypes'
import type {
  Game,
  PlayResult,
  GameStats,
  BuyOption,
  BetRequest,
  BetEvent,
  BetEventDetail,
  PrizeDef,
  GameRecord,
  SynthesisTarget,
  MedalNode,
  MedalShard,
  FlipState,
  BigSmallState,
  BigSmallChat,
  BlackjackResult,
  FarmState,
  GobangState,
  FriendTradeState,
  FishingState,
  City13WheelState,
  City13DiceState,
} from './entertainmentTypes'
import type { Site } from './site'

export const entertainmentApi = {
  /** 某站点可玩的游戏 */
  games: (siteId: number) =>
    http.get<{ games: Game[] }>(`/entertainment/${siteId}/games`).then((r) => r.data.games ?? []),
  /** 玩一次 */
  play: (siteId: number, game: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/play`, { game }).then((r) => r.data),
  /** 连抽（仅 LongPT 大转盘等支持） */
  playMulti: (siteId: number, game: string, count: number) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/play-multi`, { count }).then((r) => r.data),
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
    http.get<{ prizes: PrizeDef[] }>(`/entertainment/${siteId}/games/${game}/prizes`).then((r) => r.data.prizes ?? []),
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
    http
      .get<{ nodes: MedalNode[] }>(`/entertainment/${siteId}/games/${game}/medal-tree`)
      .then((r) => r.data.nodes ?? []),
  /** 阳爻/阴爻碎片余额 */
  medalShards: (siteId: number, game: string) =>
    http.get<{ shards: MedalShard }>(`/entertainment/${siteId}/games/${game}/medal-shards`).then((r) => r.data.shards),
  /** 碎片合成（太极/两仪/四象） */
  composeMedal: (siteId: number, game: string, medalId: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/compose`, { medal_id: medalId }).then((r) => r.data),
  /** 大勋章合成（八卦/六十四卦，耗魔力+子勋章） */
  composeBigMedal: (siteId: number, game: string, medalId: string) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/compose-big`, { medal_id: medalId })
      .then((r) => r.data),
  /** 阴阳转换（yi_yang>0 阴→阳，<0 阳→阴） */
  convertShard: (siteId: number, game: string, yiYang: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/convert-shard`, { yi_yang: yiYang })
      .then((r) => r.data),
  /** 农场全景（NovaHD 魔力农场） */
  farm: (siteId: number, game: string) =>
    http.get<FarmState>(`/entertainment/${siteId}/games/${game}/farm`).then((r) => r.data),
  /** 种植/养殖（section=crop→plant / animal→breed） */
  plant: (siteId: number, game: string, section: string, itemId: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/plant`, { section, item_id: itemId })
      .then((r) => r.data),
  /** 收获单个 */
  harvest: (siteId: number, game: string, section: string, itemId: number) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/harvest`, { section, item_id: itemId })
      .then((r) => r.data),
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
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-task`, { action, task_id: taskId })
      .then((r) => r.data),
  /** 猜大小使用道具 */
  bigsmallItem: (siteId: number, game: string, tag: string) =>
    http.post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-item`, { tag }).then((r) => r.data),
  /** 猜大小红包（send/claim/info） */
  bigsmallRedpacket: (siteId: number, game: string, action: string, extra: Record<string, string>) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/bigsmall-redpacket`, { action, extra })
      .then((r) => r.data),
  /** 二十一点动作（new/hit/stand/double） */
  blackjack: (siteId: number, game: string, action: string) =>
    http.post<BlackjackResult>(`/entertainment/${siteId}/games/${game}/blackjack`, { action }).then((r) => r.data),
  /** 五子棋全景快照（蜂巢：对局/蜂蜜汇总/周榜） */
  gobangState: (siteId: number, game: string) =>
    http.get<GobangState>(`/entertainment/${siteId}/games/${game}/gobang-state`).then((r) => r.data),
  /** 五子棋动作（create/choose-color/move/undo/forfeit/retry-ai；params 透传站点） */
  gobangAction: (siteId: number, game: string, action: string, params: Record<string, unknown>) =>
    http
      .post<PlayResult>(`/entertainment/${siteId}/games/${game}/gobang-action`, { action, params })
      .then((r) => r.data),
  /** 好友买卖全景快照（PANDA：身价/主人/奴隶/收益/任务/动态） */
  friendTradeState: (siteId: number, game: string) =>
    http
      .get<FriendTradeState>(`/entertainment/${siteId}/games/${game}/friendtrade-state`)
      .then((r) => r.data),
  /** 钓鱼全景快照（思齐：钓次/装备/图鉴） */
  fishingState: (siteId: number, game: string) =>
    http.get<FishingState>(`/entertainment/${siteId}/games/${game}/fishing-state`).then((r) => r.data),
  /** 13City 大转盘全景 */
  city13WheelState: (siteId: number, game: string) =>
    http
      .get<City13WheelState>(`/entertainment/${siteId}/games/${game}/city13-wheel-state`)
      .then((r) => r.data),
  /** 13City 幸运骰子全景 */
  city13DiceState: (siteId: number, game: string) =>
    http
      .get<City13DiceState>(`/entertainment/${siteId}/games/${game}/city13-dice-state`)
      .then((r) => r.data),
  /** 站点列表（含各站已适配娱乐项目数 game_count，独立于通用 /sites） */
  sites: () => http.get<{ sites: Site[] }>('/entertainment').then((r) => r.data.sites ?? []),
}
