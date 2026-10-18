package chain

// #407 抓取成功但没存住：必须与「抓取失败」可区分。

import (
	"errors"

	"github.com/hxpt/ptpatronus/internal/errtext"
)

// ErrSnapshotNotPersisted 抓到数据、但写库失败（BatchUpsert 事务回滚）。
//
// 这条错误此前只 slog.Warn 落进日志，界面上一切正常：
//   - assessment 仍返回一张**不含 Error** 的正常卡片，并把新数据写进内存缓存，
//     于是「本次看到的考核状态」和「库里实际存着的」从此对不上，且缓存 TTL 内
//     反复刷新都会显示同一份没存住的数据；
//   - medal 同理，errMsg 仍是空串，前端把该站当正常站点计入总数。
//
// 用户看到的是「成功」，实际下一次缓存过期后数据回退到旧快照，且没有任何线索
// 指向「库写不进去」（磁盘满 / SQLite busy / 表锁）。这不是可读性问题，是假绿。
var ErrSnapshotNotPersisted = errors.New("抓取结果未能写入数据库")

// snapshotPersistErrText 组装给用户看的原因。
//
// 措辞上刻意不说成「抓取失败」：网络那次确实成功了，用户刷新只会再成功一次、
// 然后再失败一次，真实原因是写不进去，把人引向「再试一次」是错的。
// 也不只说「稍后重试」，因为那同样指向错误方向 —— 下一步该看磁盘与日志。
//
// 「库里仍是上一次的」这句是给用户的判据：刷新页面会看到状态退回旧值，
// 那就说明这一条说的是真的，不是站点数据本来就这样。
func snapshotPersistErrText(err error) string {
	reason := errtext.Text(err)
	if reason == "" {
		reason = "未返回原因"
	}
	return ErrSnapshotNotPersisted.Error() +
		"：本次抓到的数据没能存进数据库，库里仍是上一次的（可能是磁盘已满或数据库被占用）；" +
		"界面显示的这份刷新后就会变回旧值，请检查后端日志与磁盘空间后重试（" + reason + "）"
}
