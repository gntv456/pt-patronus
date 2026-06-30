package ws

import (
	"encoding/json"
	"testing"
)

func TestEncodeHRAlert(t *testing.T) {
	payload, err := EncodeHRAlert([]HRAlert{
		{Site: "好学", Title: "Some Torrent", Status: "未达标", Deadline: "10:00:00"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "hr-alert" {
		t.Fatalf("type=%s, want hr-alert", m.Type)
	}
	// Data 是 any（json 解出为 []interface{}），重新序列化后解码到 []HRAlert。
	raw, _ := json.Marshal(m.Data)
	var alerts []HRAlert
	if err := json.Unmarshal(raw, &alerts); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Site != "好学" || alerts[0].Status != "未达标" {
		t.Fatalf("unexpected: %+v", alerts)
	}
}

// Message.Data 改 any 后，torrents 消息仍正常编码。
func TestTorrentsMessageEncodesAfterAnyData(t *testing.T) {
	payload, err := json.Marshal(Message{Type: "torrents", Data: []TorrentSnapshot{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "torrents" {
		t.Fatalf("type=%s, want torrents", m.Type)
	}
}

func TestEncodeCookieAlert(t *testing.T) {
	payload, err := EncodeCookieAlert([]string{"好学", "馒头"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m.Type != "cookie-alert" {
		t.Fatalf("type=%s, want cookie-alert", m.Type)
	}
	raw, _ := json.Marshal(m.Data)
	var sites []string
	if err := json.Unmarshal(raw, &sites); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(sites) != 2 || sites[0] != "好学" {
		t.Fatalf("unexpected: %+v", sites)
	}
}
