// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// 对应后端 GET /auth/oidc/config（脱敏视图）。
export interface OIDCConfig {
  enabled: boolean
  issuer: string
  client_id: string
  client_secret_configured: boolean
  scopes: string
  display_name: string
  frontend_redirect: string
}

// 管理员提交（client_secret 空串=保留旧值）。
export interface OIDCConfigInput {
  enabled: boolean
  issuer: string
  client_id: string
  client_secret: string
  scopes: string
  display_name: string
  frontend_redirect: string
}

export const oidcApi = {
  getConfig: () => http.get<OIDCConfig>('/auth/oidc/config').then((r) => r.data),
  setConfig: (cfg: OIDCConfigInput) => http.put('/auth/oidc/config', cfg).then((r) => r.data),
}
