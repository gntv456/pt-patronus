package subtitle

import (
	"sync"

	"github.com/longbridgeapp/opencc"
)

// 繁简转换用 OpenCC（纯 Go，内嵌字典）。两个 profile 各缓存一个实例（New 会加载字典，
// 重复构造昂贵；Convert 读字典并发安全）。
var (
	t2sOnce sync.Once
	t2sInst *opencc.OpenCC
	t2sErr  error

	s2tOnce sync.Once
	s2tInst *opencc.OpenCC
	s2tErr  error
)

func t2s() (*opencc.OpenCC, error) {
	t2sOnce.Do(func() { t2sInst, t2sErr = opencc.New("t2s") })
	return t2sInst, t2sErr
}

func s2t() (*opencc.OpenCC, error) {
	s2tOnce.Do(func() { s2tInst, s2tErr = opencc.New("s2t") })
	return s2tInst, s2tErr
}

// Convert 繁简转换。target="chs"→繁转简，"cht"→简转繁，""/其他→原样（不转换）。
// 仅对文本字幕有意义（图片字幕 .sup/.sub+idx 不转）。
func Convert(text, target string) (string, error) {
	switch target {
	case "chs":
		cc, err := t2s()
		if err != nil {
			return text, err
		}
		return cc.Convert(text)
	case "cht":
		cc, err := s2t()
		if err != nil {
			return text, err
		}
		return cc.Convert(text)
	}
	return text, nil
}
