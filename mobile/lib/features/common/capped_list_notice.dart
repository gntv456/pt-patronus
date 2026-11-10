// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// capped_list_notice.dart —— 「有单页上限的清单」的统一判定。
//
// 移动端已经有好几处这类清单（趋势采样、RSS 条目、整理记录、种子史、运行史…），
// 每处都得回答同一个问题：这次拿到的到底是不是全部？
// 判定抄第二遍一定会漂移（一处忘记处理「总数读不出」，界面就写出 共 0 次），
// 所以这里只留判定，句式由各调用方给。

/// - 未截断：空串（一次拿全不该打扰用户）
/// - 截断但总数读不出：走 [unknownTotal]，绝不把缺失渲染成 0
/// - 后端说截断、总数却不大于已显示数：空串（反过来谎报「还有更多」同样是说谎）
/// - 其余：走 [knownTotal]
String cappedListNotice({
  required int shown,
  int? total,
  required bool truncated,
  required String Function(int shown) unknownTotal,
  required String Function(int shown, int total) knownTotal,
}) {
  if (!truncated) return '';
  if (total == null) return unknownTotal(shown);
  if (total <= shown) return '';
  return knownTotal(shown, total);
}
