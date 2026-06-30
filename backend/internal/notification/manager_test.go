package notification

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// successChannel 总是成功。
type successChannel struct{ name string }

func (s successChannel) Name() string                              { return s.name }
func (s successChannel) Send(ctx context.Context, m Message) error { return nil }

// failChannel 总是失败。
type failChannel struct{ name string }

func (f failChannel) Name() string                              { return f.name }
func (f failChannel) Send(ctx context.Context, m Message) error { return errFail }

var errFail = errors.New("fail")

func TestManagerFanOutIsolation(t *testing.T) {
	m := NewManager(successChannel{"a"}, failChannel{"b"}, successChannel{"c"})
	res := m.Notify(context.Background(), Message{Title: "hi"})
	if len(res) != 3 {
		t.Fatalf("expected 3 results, got %d", len(res))
	}
	byName := map[string]SendResult{}
	for _, r := range res {
		byName[r.Channel] = r
	}
	if !byName["a"].Ok || !byName["c"].Ok {
		t.Error("a/c should succeed")
	}
	if byName["b"].Ok {
		t.Error("b should fail but succeed")
	}
}

func TestManagerChannels(t *testing.T) {
	m := NewManager(successChannel{"a"}, successChannel{"b"})
	names := m.Channels()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("channels = %v", names)
	}
}

// TestManagerReplace：Replace 整体替换通道切片，Channels/Notify 立即反映。
func TestManagerReplace(t *testing.T) {
	m := NewManager(successChannel{"a"})
	if got := m.Channels(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("before replace: %v", got)
	}
	m.Replace([]Channel{successChannel{"x"}, successChannel{"y"}, failChannel{"z"}})
	if got := m.Channels(); len(got) != 3 || got[0] != "x" || got[2] != "z" {
		t.Errorf("after replace: %v", got)
	}
	res := m.Notify(context.Background(), Message{Title: "hi"})
	if len(res) != 3 || !res[0].Ok || res[2].Ok {
		t.Errorf("notify after replace: %+v", res)
	}
	// Replace 为空 → 清空。
	m.Replace(nil)
	if got := m.Channels(); len(got) != 0 {
		t.Errorf("after replace nil: %v", got)
	}
	if res := m.Notify(context.Background(), Message{Title: "hi"}); res != nil {
		t.Errorf("notify empty manager should return nil, got %+v", res)
	}
}

func TestWebhookSend(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		got = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL)
	if err := w.Send(context.Background(), Message{Title: "T", Body: "B", Level: "info"}); err != nil {
		t.Fatal(err)
	}
	if got == "" || !contains(got, "\"title\":\"T\"") {
		t.Errorf("webhook payload = %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
