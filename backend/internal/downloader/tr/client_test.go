package tr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTRSessionHandshakeAndList(t *testing.T) {
	const sid = "SID-xyz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transmission/rpc" {
			w.WriteHeader(404)
			return
		}
		// 鉴权缺失 → 409 给会话 id
		if r.Header.Get("X-Transmission-Session-Id") != sid {
			w.Header().Set("X-Transmission-Session-Id", sid)
			w.WriteHeader(409)
			return
		}
		// 已握手：按 method 分流
		switch {
		case r.URL.Query().Get("m") == "session-get" || r.Header.Get("X-Test-Method") == "session-get":
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"downloadSpeed":1000,"uploadSpeed":2000}}`))
		default:
			_, _ = w.Write([]byte(`{"result":"success","arguments":{"torrents":[
				{"id":1,"name":"T1","hashString":"abc","totalSize":1000,"status":4,"percentDone":0.5,"uploadRatio":1.2,"rateDownload":500,"rateUpload":100,"downloadDir":"/data","labels":["seed"],"addedDate":1700000000}
			]}}`))
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "u", "p")

	// TestConnection（触发 409 握手 + session-get）
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatalf("test conn: %v", err)
	}
	c.mu.Lock()
	gotSID := c.sessionID
	c.mu.Unlock()
	if gotSID != sid {
		t.Errorf("session id = %q, want %q", gotSID, sid)
	}

	// ListTorrents
	ts, err := c.ListTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].Name != "T1" || ts[0].State != "downloading" || ts[0].UPRate != 100 {
		t.Errorf("list = %+v", ts)
	}
}
