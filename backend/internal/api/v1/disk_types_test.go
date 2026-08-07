package v1

import (
	"encoding/json"
	"testing"
)

func TestLsblkBoolAcceptsBoolAndString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`true`, "1"},
		{`false`, "0"},
		{`"1"`, "1"},
		{`"0"`, "0"},
		{`"true"`, "1"},
		{`"FALSE"`, "0"},
		{`null`, ""},
		{`""`, "0"},
	}
	for _, c := range cases {
		var b lsblkBool
		if err := json.Unmarshal([]byte(c.in), &b); err != nil {
			t.Fatalf("Unmarshal(%s) error: %v", c.in, err)
		}
		if string(b) != c.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", c.in, b, c.want)
		}
	}
}

func TestDiskTreeParsesBoolRota(t *testing.T) {
	raw := `{
		"blockdevices": [
			{"name":"sda","path":"/dev/sda","type":"disk","size":1000,"rota":true,"ro":false,
			 "children":[{"name":"sda1","type":"part","size":500,"mountpoint":"/"}]}
		]
	}`
	var tree diskTreeResp
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		t.Fatalf("parse bool-form lsblk error: %v", err)
	}
	if string(tree.BlockDevices[0].Rota) != "1" {
		t.Errorf("Rota = %q, want 1", tree.BlockDevices[0].Rota)
	}
	if string(tree.BlockDevices[0].ReadOnly) != "0" {
		t.Errorf("ReadOnly = %q, want 0", tree.BlockDevices[0].ReadOnly)
	}
}

func TestDiskTreeParsesStringRota(t *testing.T) {
	raw := `{
		"blockdevices": [
			{"name":"nvme0n1","path":"/dev/nvme0n1","type":"disk","size":2000,"rota":"0","ro":"0"}
		]
	}`
	var tree diskTreeResp
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		t.Fatalf("parse string-form lsblk error: %v", err)
	}
	if string(tree.BlockDevices[0].Rota) != "0" {
		t.Errorf("Rota = %q, want 0", tree.BlockDevices[0].Rota)
	}
}