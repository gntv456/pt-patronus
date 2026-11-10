// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"bytes"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefinitionUnknownKeys 用严格模式再解一次，返回 YAML 里那些「结构体不认识」的键。
//
// 为什么单独做这件事：yaml.Unmarshal 默认对未知键**不报错也不吭声**——键名打错的整块配置
// 会直接蒸发，文件照样加载成功、站点数照样 +1，界面上只是「这个站什么规则都没有」。
// （本机实测：探针 fixture 把顶层 selectors: 误写成 search: 下嵌套，306 文件的体检与抓取
// 都抓不到任何东西，正是这一形。）
//
// 只诊断、不否决：一个键错不该让用户自己放的外部目录被整批退回内置定义（loadDefinitionsFS
// 现在的失败语义是「parse 出错 ⇒ 整个目录 return err」，严格模式若走那条路会放大后果）。
func DefinitionUnknownKeys(data []byte) []string {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var def SiteMetadata
	err := dec.Decode(&def)
	if err == nil {
		return nil
	}
	var unknown []string
	for _, line := range strings.Split(err.Error(), "\n") {
		// yaml.TypeError 的行形如：  line 3: field nam not found in type site.SiteMetadata
		if strings.Contains(line, "not found in type") {
			unknown = append(unknown, strings.TrimSpace(line))
		}
	}
	return unknown
}

// warnUnknownDefinitionKeys 逐条 Warn + 记账（界面靠 Diagnostics() 读），加载流程照常继续。
func (r *Registry) warnUnknownDefinitionKeys(file string, data []byte) {
	for _, msg := range DefinitionUnknownKeys(data) {
		slog.Warn("site definition has a key that is ignored by the loader; that block of config does nothing",
			"file", file, "detail", msg)
		r.recordDiag(DefinitionDiagnostic{Kind: DiagUnknownKey, File: file,
			Detail: "这里有加载器不认识的键，那块配置不会生效：" + msg})
	}
}
