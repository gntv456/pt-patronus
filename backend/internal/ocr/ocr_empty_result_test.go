// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ocr

import (
	"errors"
	"testing"
)

// #312 三种响应形态必须共用同一条判据：结果为空 = 什么都没认出来，不能算识别成功。
// paddle 分支早就判 "ocr 空结果"，simple/ddddoc 却把 clean("") 原样当成功返回 ⇒
// 服务端 200 {"text":""} → 扩展 ocrCaptcha 回 {ok:true,text:''} → content 层把**空串**
// 填进站点登录验证码框。用户看到的是「识别错了/站点拒了」，实际是「OCR 什么都没认出来」，
// 两者下一步完全不同（前者该换模式/该怀疑图，后者该人工输入）。

func TestParseEmptyResultIsFailureInAllModes(t *testing.T) {
	cases := []struct{ name, mode, body string }{
		{"simple 空 text", "simple", `{"text":""}`},
		{"simple 只有空白", "simple", `{"text":"  \n "}`},
		{"simple 没带 text 键（服务改版/回的是错误信封）", "simple", `{"code":500,"msg":"bad image"}`},
		{"ddddoc 空 data", "ddddoc", `{"data":{"data":""}}`},
		{"ddddoc 没带 data 键", "ddddoc", `{"code":500}`},
		// 下面两条是同一函数里既有的正确形状：拿来当 A/B 对照，证明这不是设计分歧而是漏抄。
		{"paddle results 空", "paddle", `{"results":[]}`},
		{"paddle text 空串", "paddle", `{"results":[[{"text":""}]]}`},
		{"数组兜底 text 空串", "paddle", `[{"text":""}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parse([]byte(c.body), c.mode)
			if !errors.Is(err, ErrEmptyResult) {
				t.Errorf("空结果必须判成失败（ErrEmptyResult），实际 text=%q err=%v", got, err)
			}
			if got != "" {
				t.Errorf("失败时不许回文本，实际 %q", got)
			}
		})
	}
}

// 正控：真认出来了就照原样回（只去首尾空白，保留大小写）。
func TestParseKeepsRecognizedText(t *testing.T) {
	cases := []struct{ name, mode, body, want string }{
		{"simple", "simple", `{"text":" aB3 "}`, "aB3"},
		{"ddddoc", "ddddoc", `{"data":{"data":"7x9K"}}`, "7x9K"},
		{"paddle", "paddle", `{"results":[[{"text":"Qw4p","confidence":0.9}]]}`, "Qw4p"},
		{"数组兜底", "paddle", `[{"text":"Zz8"}]`, "Zz8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parse([]byte(c.body), c.mode)
			if err != nil {
				t.Errorf("识别成功不该报错：%v", err)
			}
			if got != c.want {
				t.Errorf("文本：期望 %q，实际 %q", c.want, got)
			}
		})
	}
}
