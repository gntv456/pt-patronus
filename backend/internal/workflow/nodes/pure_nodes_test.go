// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

func TestSwitchNodeBranches(t *testing.T) {
	node := NewSwitchNode()
	tests := []struct {
		name  string
		rules string
		vars  map[string]any
		count int
		want  string
	}{
		{name: "numeric count", rules: "large,count,ge,3", count: 3, want: "large"},
		{name: "variable contains", rules: "movie,var.kind,contains,film", vars: map[string]any{"kind": "film-4k"}, want: "movie"},
		{name: "string equality", rules: "ready,var.state,eq,ok", vars: map[string]any{"state": "ok"}, want: "ready"},
		{name: "invalid rules use default", rules: "broken\nnone,count,gt,9", count: 1, want: "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecx := &workflow.Context{Variables: tt.vars, Torrents: make([]site.Torrent, tt.count)}
			err := node.Execute(context.Background(), map[string]any{"rules": tt.rules, "default_branch": "fallback"}, ecx)
			if err != nil {
				t.Fatal(err)
			}
			if len(ecx.Next) != 1 || ecx.Next[0] != tt.want {
				t.Fatalf("next = %v, want %q", ecx.Next, tt.want)
			}
		})
	}
}

func TestMathNodeOperationsAndErrors(t *testing.T) {
	node := NewMathNode()
	tests := []struct {
		name    string
		op      string
		left    string
		right   string
		want    any
		wantErr bool
	}{
		{name: "add", op: "add", left: "2", right: "3", want: int64(5)},
		{name: "subtract", op: "sub", left: "7", right: "2", want: int64(5)},
		{name: "multiply", op: "mul", left: "2.5", right: "2", want: int64(5)},
		{name: "divide fraction", op: "div", left: "5", right: "2", want: 2.5},
		{name: "concat template", op: "concat", left: "{{var.prefix}}", right: "-{{count}}", want: "item-2"},
		{name: "divide zero", op: "div", left: "1", right: "0", wantErr: true},
		{name: "not numeric", op: "add", left: "x", right: "1", wantErr: true},
		{name: "unknown", op: "pow", left: "2", right: "3", wantErr: true},
		{name: "missing result", op: "add", left: "1", right: "1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecx := &workflow.Context{Variables: map[string]any{"prefix": "item"}, Torrents: make([]site.Torrent, 2)}
			key := "result"
			if tt.name == "missing result" {
				key = ""
			}
			err := node.Execute(context.Background(), map[string]any{"operation": tt.op, "left": tt.left, "right": tt.right, "result_var": key}, ecx)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := ecx.Variables[key]
			if wantFloat, ok := tt.want.(float64); ok {
				gotFloat, ok := got.(float64)
				if !ok || math.Abs(gotFloat-wantFloat) > 0.0001 {
					t.Fatalf("result = %#v, want %v", got, wantFloat)
				}
			} else if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExtractFieldsNode(t *testing.T) {
	node := NewExtractFieldsNode()
	t.Run("named groups", func(t *testing.T) {
		ecx := &workflow.Context{Content: "name=alice id=42"}
		err := node.Execute(context.Background(), map[string]any{"pattern": `name=(?P<name>\w+) id=(?P<id>\d+)`}, ecx)
		if err != nil {
			t.Fatal(err)
		}
		if ecx.Variables["name"] != "alice" || ecx.Variables["id"] != "42" {
			t.Fatalf("variables = %v", ecx.Variables)
		}
	})
	t.Run("no match", func(t *testing.T) {
		ecx := &workflow.Context{Content: "none"}
		if err := node.Execute(context.Background(), map[string]any{"pattern": `id=(?P<id>\d+)`}, ecx); err != nil {
			t.Fatal(err)
		}
		if len(ecx.Messages) != 1 || ecx.Variables != nil {
			t.Fatalf("messages = %v", ecx.Messages)
		}
	})
	for name, pattern := range map[string]string{"empty": "", "invalid": "["} {
		t.Run(name, func(t *testing.T) {
			if err := node.Execute(context.Background(), map[string]any{"pattern": pattern}, &workflow.Context{}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPureNodeHelpers(t *testing.T) {
	if !matchResolution("Movie.2160p", "4K") || !matchResolution("Movie.4K", "2160p") || matchResolution("Movie.1080p", "2160p") {
		t.Fatal("resolution aliases are incorrect")
	}
	if !matchAny("Movie REMUX", "web|remux") || matchNone("Movie CAM", "cam|ts") {
		t.Fatal("keyword matching is incorrect")
	}
	ecx := &workflow.Context{
		Content:   "payload",
		Variables: map[string]any{"name": "demo"},
		Torrents:  []site.Torrent{{Title: "Title", Size: "1 GiB", SiteName: "Site", DownloadURL: "down", DetailURL: "detail"}},
	}
	got := render("{{var.name}}/{{count}}/{{content}}/{{torrents.0.title}}/{{torrents.0.size}}/{{torrents.0.site}}/{{torrents.0.url}}/{{torrents.0.detail_url}}", ecx)
	if got != "demo/1/payload/Title/1 GiB/Site/down/detail" {
		t.Fatalf("render = %q", got)
	}
	if parseShareRatio("") != -1 || parseShareRatio("-2") != -2 || parseShareRatio("bad") != -1 || parseShareRatio("1.5") != 1.5 {
		t.Fatal("share ratio parsing is incorrect")
	}
	if got := nonEmptyLines(" a \n\n b "); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("lines = %v", got)
	}
	if ifElse(true, "yes", "no") != "yes" || ifElse(false, "yes", "no") != "no" {
		t.Fatal("ifElse is incorrect")
	}
}
