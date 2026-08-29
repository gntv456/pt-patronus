import { http } from './http'

// LLM 网关配置（脱敏：access_token 仅 _configured）。
export interface LLMGatewayConfig {
  enabled: boolean
  access_token_configured: boolean
  allowed_models: string[]
  max_concurrency: number
}

export const llmGatewayApi = {
  getConfig: () => http.get<LLMGatewayConfig>('/llmgateway/config').then((r) => r.data),
  setConfig: (cfg: { enabled: boolean; access_token?: string; allowed_models: string[]; max_concurrency: number }) =>
    http.put('/llmgateway/config', cfg).then((r) => r.data),
}
