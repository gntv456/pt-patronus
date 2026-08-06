package site

// EnsureDefaults 按 schema 填充站点定义的空段（不改原 def，返回副本）。
//
// 单入口分发（dispatch）：被 ~20 处调用（chain 层 + 全部 capability 适配器），
// 签名固定不变。内部按 def.Schema 路由：
//   - ensureMinimalDefaults：所有 schema 共享的路径默认（搜索入口 / HR 路径）。
//   - nexusphp（含空 schema，向后兼容现有 20 站与测试）：ensureNexusPHPDefaults 填选择器/capability 默认。
//   - 其它 schema（unit3d/gazelle/...）：仅通用项，不注入 NexusPHP 选择器，避免污染
//     （非 NexusPHP 站填入 table.torrents tr / font.color_* 会抓错页面）。各 schema 专属
//     默认在该 schema 适配器落地时补 ensureXxxDefaults 分支。
//
// 幂等：同一 def 多次调用结果一致（回归测试 TestEnsureDefaultsIdempotent 钉死）。
func EnsureDefaults(def *SiteMetadata) *SiteMetadata {
	cp := *def
	ensureMinimalDefaults(&cp)
	switch cp.Schema {
	case "", "nexusphp":
		ensureNexusPHPDefaults(&cp)
	case "tnode":
		ensureTNodeDefaults(&cp)
	case "unit3d":
		ensureUnit3DDefaults(&cp)
	case "gazelle":
		ensureGazelleDefaults(&cp)
	case "luminance":
		ensureLuminanceDefaults(&cp)
	case "tcg":
		ensureTCGDefaults(&cp)
	case "smallhorse":
		ensureSmallHorseDefaults(&cp)
	case "mteam":
		ensureMTeamDefaults(&cp)
	case "rartracker":
		ensureRartrackerDefaults(&cp)
	default:
		// 非 nexusphp：仅通用项，保留站点 YAML 自定义，不注入 NexusPHP 默认。
	}
	return &cp
}

// EnsureLoginDefaults 填 LoginDef 的空字段为好学/NexusPHP 标准默认值（不改原 def，返回副本）。
func EnsureLoginDefaults(def *SiteMetadata) *SiteMetadata {
	def = EnsureDefaults(def)
	if !def.Login.Enabled {
		return def
	}
	cp := *def
	if cp.Login.PagePath == "" {
		cp.Login.PagePath = "login.php"
	}
	if cp.Login.ActionPath == "" {
		cp.Login.ActionPath = "takelogin.php"
	}
	fm := &cp.Login.Form
	if fm.Username == "" {
		fm.Username = "username"
	}
	if fm.Password == "" {
		fm.Password = "password"
	}
	if fm.ImageHash == "" {
		fm.ImageHash = "imagehash"
	}
	if fm.ImageString == "" {
		fm.ImageString = "imagestring"
	}
	if fm.TwoStepCode == "" {
		fm.TwoStepCode = "two_step_code"
	}
	if cp.Login.Captcha.GenPath == "" {
		cp.Login.Captcha.GenPath = "image.php"
	}
	if cp.Login.Captcha.HashInputSelector == "" {
		cp.Login.Captcha.HashInputSelector = `input[name="imagehash"]`
	}
	return &cp
}

// applyInfoDefaults 合并 Freeleech/Announcement 默认值：站点未显式配置（Enabled=false 且 Path 空）
// 时整体取默认（开启 + 默认路径）；否则仅补空 Path。站点 YAML 可用 enabled:false 关闭。
func applyInfoDefaults(curEnabled bool, curPath string, def FreeleechDef) (bool, string) {
	if !curEnabled && curPath == "" {
		return def.Enabled, def.Path
	}
	if curPath == "" {
		curPath = def.Path
	}
	return curEnabled, curPath
}

// mergeDefaultFields 把 def 里「site 未定义的键」逐个补入，返回新 map（不覆盖 site 已配的键，不污染 site 原 map）。
// 用于 NexusPHP UserInfo.Fields / DetailFields 的默认值逐键合并——站点只覆盖个别键（如 bonus），
// 其余键仍继承默认（uploaded/downloaded/ratio/...）。新建 map 是因 EnsureDefaults 浅拷贝 struct，
// map 字段仍与原 def 共享，原地改写会污染 Registry 里的共享定义。
func mergeDefaultFields(site, def map[string]ElementQuery) map[string]ElementQuery {
	if len(def) == 0 {
		return site
	}
	out := make(map[string]ElementQuery, len(site)+len(def))
	for k, v := range site {
		out[k] = v
	}
	for k, v := range def {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func mergeDefaultUserInfoPages(sitePages, defaultPages []UserInfoPageDef) []UserInfoPageDef {
	if len(defaultPages) == 0 {
		return sitePages
	}
	covered := map[string]bool{}
	for _, page := range sitePages {
		for key := range page.Fields {
			covered[key] = true
		}
		for _, key := range page.UseFields {
			covered[key] = true
		}
	}
	out := append([]UserInfoPageDef{}, sitePages...)
	for _, page := range defaultPages {
		needed := false
		for key := range page.Fields {
			if !covered[key] {
				needed = true
				covered[key] = true
			}
		}
		for _, key := range page.UseFields {
			if !covered[key] {
				needed = true
				covered[key] = true
			}
		}
		if needed {
			out = append(out, page)
		}
	}
	return out
}
