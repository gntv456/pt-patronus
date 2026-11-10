// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// Telegram bot 配置（脱敏：bot_token 仅 _configured）。
export interface TgBotConfig {
  enabled: boolean
  bot_token_configured: boolean
  allowed_user_ids: number[]
  allow_group_chat: boolean
  api_url: string
}

export const tgBotApi = {
  getConfig: () => http.get<TgBotConfig>('/tgbot/config').then((r) => r.data),
  setConfig: (cfg: {
    enabled: boolean
    bot_token?: string
    allowed_user_ids: number[]
    allow_group_chat: boolean
    api_url: string
  }) => http.put('/tgbot/config', cfg).then((r) => r.data),
}
