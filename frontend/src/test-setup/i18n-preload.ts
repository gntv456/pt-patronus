// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// i18n 消息表预载：composable/ops 层的文案已抽进翻译表，模块级 helper 用
// i18n.global.t 取文案；测试环境没人跑 i18nBoot（懒加载），t() 会退回 key 本身，
// 断言中文文案的用例全数误报。这里同步 import zh 表并直接挂载。
import { i18n } from '@/i18n'
import zhCN from '@/i18n/locales/zh-CN'

i18n.global.setLocaleMessage('zh-CN', zhCN)
