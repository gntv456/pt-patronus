package v1

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

// TestSubReqBindingMax 验证 API 层 subReq 的 binding max 生效（代表性，其余 rssReq/webhookReq/wfReq 同模式）。
func TestSubReqBindingMax(t *testing.T) {
	v := validator.New()
	v.SetTagName("binding")
	if err := v.Struct(subReq{Source: "tmdb", SourceID: "123", Title: "正常标题", Type: "movie"}); err != nil {
		t.Fatalf("normal subReq should pass: %v", err)
	}
	cases := []struct {
		name string
		in   subReq
	}{
		{"Title>255", subReq{Title: strings.Repeat("a", 256)}},
		{"Source>16", subReq{Source: strings.Repeat("a", 17)}},
		{"PosterURL>512", subReq{PosterURL: strings.Repeat("a", 513)}},
	}
	for _, tc := range cases {
		if err := v.Struct(tc.in); err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
	}
}
