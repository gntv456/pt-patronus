package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// ImportPTD 从 PT-Depiler 浏览器扩展导出的 .ptd 备份（ZIP）导入站点。
//
// 移植自旧项目（F:\PT-admin\PTPatronus）的 PtdBackupImportService。.ptd 结构：
//   - manifest.json（必需）：{encryption, version, files:{metadata:{name}, cookies:{name}}}
//   - metadata 文件：{sites:{id:{url,merge,inputSetting,isOffline}}, siteNameMap, lastUserInfo, siteHostMap}
//   - cookies 文件：{host:[{name,value}]}
//
// 加密备份（manifest.encryption=true）拒绝；重复站点按注册域跳过（CreateMany 统一去重，与自身/MoviePilot 导入一致）。
// 与旧项目唯一差异：cookie 拼成 header 串（"k=v; k=v"）而非 JSON map（适配新 SiteInput.Cookie）。
func (s *Service) ImportPTD(ctx context.Context, zipBytes []byte) (int, error) {
	entries, err := readPTDZipEntries(zipBytes)
	if err != nil {
		return 0, err
	}
	manifestRaw, ok := entries["manifest.json"]
	if !ok {
		return 0, errors.New("不是合法的 PT-Depiler (.ptd) 备份：缺少 manifest.json")
	}
	manifest := parseJSON(manifestRaw)
	if manifest == nil {
		return 0, errors.New("PTD manifest.json 解析失败")
	}
	if ptdBool(manifest, "encryption") {
		return 0, errors.New("PT-Depiler 备份已加密，请先在 PT-Depiler 中导出未加密的备份再导入")
	}

	files, _ := manifest["files"].(map[string]any)
	metaName := ptdEntryName(files, "metadata", "metadata.json")
	cookieName := ptdEntryName(files, "cookies", "cookies.json")

	metadata := parseJSON(entries[metaName])
	if metadata == nil {
		return 0, errors.New("PTD 备份缺少 metadata（" + metaName + "）")
	}
	sites, _ := metadata["sites"].(map[string]any)
	if len(sites) == 0 {
		return 0, errors.New("PTD 备份 metadata 中无站点")
	}
	siteNameMap, _ := metadata["siteNameMap"].(map[string]any)
	lastUserInfo, _ := metadata["lastUserInfo"].(map[string]any)
	siteHostMap, _ := metadata["siteHostMap"].(map[string]any)
	hostBySite := buildHostBySite(siteHostMap)
	cookies := parseJSON(entries[cookieName])

	ins := make([]oper.SiteInput, 0, len(sites))
	for ptdSiteID, siteEl := range sites {
		config, _ := siteEl.(map[string]any)
		if config == nil {
			continue
		}
		rawURL := ptdSiteURL(config)
		if rawURL == "" {
			continue // 无可用 URL 的站点跳过
		}
		baseURL := ensureURL(rawURL)

		input, _ := config["inputSetting"].(map[string]any)
		userInfo, _ := lastUserInfo[ptdSiteID].(map[string]any)

		name := ptdSiteName(siteNameMap, ptdSiteID, config)
		if name == "" {
			name = ptdSiteID
		}
		username, _ := userInfo["name"].(string)
		if username == "" {
			username = firstString(input, "username", "userName", "loginName")
		}
		// 按 URL 匹配站点定义：朱雀等 tnode 站、以及各有专属 yaml 的 nexusphp 站
		// 都挂正确定义（不再全退化为默认 nexusphp）；未命中回退 platform 推断。
		ins = append(ins, oper.SiteInput{
			Name:       name,
			Definition: s.matchDef(baseURL),
			Platform:   inferPlatform(ptdSiteID, config),
			BaseURL:    baseURL,
			Cookie:     buildCookieString(cookies, ptdSiteID, baseURL, hostBySite),
			Passkey:    firstString(input, "passkey", "passKey", "pid", "rsskey"),
			Username:   username,
			Password:   firstString(input, "password", "passWord", "loginPwd"),
			IsActive:   !ptdBool(config, "isOffline"),
		})
	}
	n, err := s.siteOp.CreateMany(ins) // 单事务批量：N 次 fsync → 1 次
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, errors.New("PTD 备份中无可导入的站点（均无有效 URL 或全部重复）")
	}
	return n, nil
}

// readPTDZipEntries 解压 .ptd（ZIP），收集所有 .json entry 为 map[文件名]UTF-8 内容。
func readPTDZipEntries(zipBytes []byte) (map[string]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("解析 .ptd 失败（非 ZIP 文件）: %w", err)
	}
	// 解压炸弹防护：单文件/总量上限 + 解压前按 UncompressedSize64 预拒
	//（防 64MB 压缩包解压成数十 GB 塞进内存导致 OOM）。
	const (
		maxPerEntry int64 = 16 << 20 // 单 entry 解压上限 16MB（站点元数据 JSON 远小于此）
		maxTotal    int64 = 64 << 20 // 全包解压累计上限 64MB
	)
	entries := make(map[string]string)
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(f.Name), ".json") {
			continue
		}
		if f.UncompressedSize64 > uint64(maxPerEntry) {
			return nil, fmt.Errorf("PTD 条目 %q 解压后过大（%d 字节，上限 %d）", f.Name, f.UncompressedSize64, maxPerEntry)
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		// LimitReader 再加 1 字节探测超限（UncompressedSize64 可被伪造，双保险）。
		data, err := io.ReadAll(io.LimitReader(rc, maxPerEntry+1))
		_ = rc.Close()
		if err != nil {
			continue
		}
		if int64(len(data)) > maxPerEntry {
			return nil, fmt.Errorf("PTD 条目 %q 解压超出单文件上限", f.Name)
		}
		total += int64(len(data))
		if total > maxTotal {
			return nil, fmt.Errorf("PTD 备份解压累计超出上限（%d 字节）", maxTotal)
		}
		entries[f.Name] = string(data)
	}
	return entries, nil
}

func parseJSON(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

// ptdEntryName 取 manifest.files[key].name，缺省 fallback。
func ptdEntryName(files map[string]any, key, fallback string) string {
	if entry, _ := files[key].(map[string]any); entry != nil {
		if name, _ := entry["name"].(string); name != "" {
			return name
		}
	}
	return fallback
}

// ptdSiteURL 取 config.url，否则 merge.urls[0]。
func ptdSiteURL(config map[string]any) string {
	if u, _ := config["url"].(string); strings.TrimSpace(u) != "" {
		return u
	}
	merge, _ := config["merge"].(map[string]any)
	urls, _ := merge["urls"].([]any)
	for _, u := range urls {
		if s, _ := u.(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// ptdSiteName 取 siteNameMap[id]，否则 merge.name。
func ptdSiteName(siteNameMap map[string]any, id string, config map[string]any) string {
	if n, _ := siteNameMap[id].(string); n != "" {
		return n
	}
	if merge, _ := config["merge"].(map[string]any); merge != nil {
		if n, _ := merge["name"].(string); n != "" {
			return n
		}
	}
	return ""
}

// ensureURL 规整：trim、去尾 /、无 scheme 补 https://。
func ensureURL(raw string) string {
	s := strings.TrimSpace(strings.TrimRight(raw, "/"))
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return s
}

// inferPlatform 按 schema 关键字 + site id 推断平台，默认 nexusphp。
// （旧项目还查 KnownSites.match(url).platform；新项目未移植 KnownSites，纯 schema 推断。）
func inferPlatform(ptdSiteID string, config map[string]any) string {
	schema := ""
	if merge, _ := config["merge"].(map[string]any); merge != nil {
		if s, _ := merge["schema"].(string); s != "" {
			schema = strings.ToLower(s)
		}
	}
	if schema == "" {
		if s, _ := config["schema"].(string); s != "" {
			schema = strings.ToLower(s)
		}
	}
	switch {
	case strings.Contains(strings.ToLower(ptdSiteID), "mteam"):
		return "mteam"
	case strings.Contains(schema, "mtorrent"):
		return "mtorrent"
	case strings.Contains(schema, "unit3d"):
		return "unit3d"
	case strings.Contains(schema, "gazelle"):
		return "gazelle"
	case strings.Contains(schema, "tnode"):
		return "tnode"
	default:
		return "nexusphp"
	}
}

// ptdBool 读 config[key]（兼容 bool / "true"/"1"/"yes" 字符串）。
func ptdBool(config map[string]any, key string) bool {
	switch v := config[key].(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(v) {
		case "true", "1", "yes":
			return true
		}
	}
	return false
}

// firstString 返回 m 中第一个非空字符串字段（key 顺序敏感）。
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, _ := m[k].(string); strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// buildHostBySite 把 siteHostMap{host: siteId} 翻转为 siteId → []host。
func buildHostBySite(siteHostMap map[string]any) map[string][]string {
	out := map[string][]string{}
	for host, v := range siteHostMap {
		siteID, _ := v.(string)
		if siteID == "" {
			continue
		}
		out[siteID] = append(out[siteID], host)
	}
	return out
}

// buildCookieString 从 cookies.json 按站点 host 候选收集 name=value，拼成 cookie header 串。
// 候选 = URL host（含去 www）+ siteHostMap[ptdSiteID] 的 host；保序去重。
func buildCookieString(cookies map[string]any, ptdSiteID, baseURL string, hostBySite map[string][]string) string {
	if len(cookies) == 0 {
		return ""
	}
	var candidates []string
	seen := map[string]bool{}
	add := func(h string) {
		h = normalizeHost(h)
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		candidates = append(candidates, h)
	}
	if host := extractHost(baseURL); host != "" {
		add(host)
		add(strings.TrimPrefix(host, "www.")) // www.abc.com → 额外匹配 abc.com
	}
	for _, h := range hostBySite[ptdSiteID] {
		add(h)
	}
	if len(candidates) == 0 {
		return ""
	}

	var collected []string
	have := map[string]bool{}
	for cookieHost, el := range cookies {
		if !hostMatches(candidates, cookieHost) {
			continue
		}
		arr, _ := el.([]any)
		for _, c := range arr {
			obj, _ := c.(map[string]any)
			if obj == nil {
				continue
			}
			name, _ := obj["name"].(string)
			if strings.TrimSpace(name) == "" || have[name] {
				continue
			}
			have[name] = true
			value, _ := obj["value"].(string)
			collected = append(collected, name+"="+value)
		}
	}
	return strings.Join(collected, "; ")
}

// hostMatches 判断 cookieHost 是否匹配任一候选 host（精确或父子域后缀）。
func hostMatches(candidates []string, rawCookieHost string) bool {
	ch := normalizeHost(rawCookieHost)
	for _, c := range candidates {
		if c == ch || strings.HasSuffix(c, "."+ch) || strings.HasSuffix(ch, "."+c) {
			return true
		}
	}
	return false
}

// normalizeHost 规整 host：去 scheme/path/port/query/fragment，小写，去首尾点。
func normalizeHost(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	for _, sep := range []string{"/", "?", "#", ":"} {
		if i := strings.Index(s, sep); i >= 0 {
			s = s[:i]
		}
	}
	return strings.Trim(s, ".")
}

// extractHost 从 URL 取 host（须含点号才算有效域名）。
func extractHost(rawURL string) string {
	h := normalizeHost(rawURL)
	if h == "" || !strings.Contains(h, ".") {
		return ""
	}
	return h
}
