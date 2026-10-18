// transfer_account.dart —— 整理记录的取数窗口文案（纯函数，口径对齐 Web useTransferHistoryOps）。
// 后端 /transfer/history 默认只给最近 200 条、封顶 500：不把窗口说出来，
// 界面就把「最近 200 次整理」画成全部记录，被挤掉的失败记录等于不存在。

/// 已显示条数、该筛选下的总条数（读不出为 null）、是否被上限截断。
String transferHistoryWindowLabel({
  required int shown,
  int? total,
  required bool truncated,
}) {
  if (!truncated) return '';
  if (total == null) {
    return '已显示最近 $shown 条整理记录，但总数读取失败——可能还有更多';
  }
  if (total <= shown) return '';
  return '仅显示最近 $shown 条整理记录（共 $total 条，更早的未加载）';
}
