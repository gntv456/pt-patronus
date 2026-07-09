package groupchat

import (
	"reflect"
	"testing"
)

func TestDispatch(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"好学PT", "groupchat.hxptHandler"},
		{"织梦", "groupchat.zmHandler"},
		{"LongPT", "groupchat.longPTHandler"},
		{"青蛙", "groupchat.qingwaHandler"},
		{"象站", "groupchat.vicomoHandler"},
		{"幸运PT", "groupchat.luckptHandler"},
		{"藏宝阁", "groupchat.cangbaoHandler"},
		{"天枢", "groupchat.dubheHandler"},
		{"Moment", "groupchat.momentHandler"},
		{"PTLGS", "groupchat.ptlgsHandler"},
		{"PTLGS-RG", "groupchat.ptlgsHandler"}, // url 域名也匹配，但名字已命中
		{"PTS", "groupchat.ptskitHandler"},
		{"13City", "groupchat.thirteenCityHandler"},
		{"某个普通 NexusPHP 站", "groupchat.nexusPHPHandler"}, // 兜底
		{"OurHome", "groupchat.nexusPHPHandler"},             // 未识别 → 兜底
	}
	for _, c := range cases {
		h := Dispatch(SiteRef{Name: c.name})
		if h == nil {
			t.Fatalf("Dispatch(%q) returned nil", c.name)
		}
		got := reflect.TypeOf(h).String()
		if got != c.want {
			t.Errorf("Dispatch(%q) = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestDispatchNoMatchForEmptyName(t *testing.T) {
	// 空名也应落到 NexusPHP 兜底(不在排除列表)
	h := Dispatch(SiteRef{Name: ""})
	if h == nil {
		t.Fatal("Dispatch(empty) should still return NexusPHP fallback, got nil")
	}
}
