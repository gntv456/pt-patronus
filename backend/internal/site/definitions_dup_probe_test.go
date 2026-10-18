package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// #330 普查：definitions 目录的两个「静默丢配置」形状
// ①同一 id 出现在两个文件 ⇒ Registry.defs[id] 被后者顶掉，前者的规则整个不存在；
// ②子目录被 `if e.IsDir() { continue }` 整批跳过 ⇒ 用户把定义放进 definitions/extra/ 就是没读。
// 本轮先只取读数（Logf），确认判据抓得到东西再决定钉成闸门。
func TestProbeDuplicateDefinitionIDsAndSubdirs(t *testing.T) {
	dir := filepath.Join("..", "..", "definitions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读不到 %s：%v", dir, err)
	}

	ids := map[string][]string{}
	nYaml := 0
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		nYaml++
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("读不到 %s：%v", name, err)
		}
		var def SiteMetadata
		if err := yaml.Unmarshal(data, &def); err != nil {
			continue // 语法错误另有判据（#329 的严格解码），这里不混栏。
		}
		if def.ID == "" {
			continue // 加载器会跳过无 ID 的文件：单独记一栏，别算进重复。
		}
		ids[def.ID] = append(ids[def.ID], name)
	}

	var dup []string
	for id, files := range ids {
		if len(files) > 1 {
			dup = append(dup, id+" <- "+strings.Join(files, ", "))
		}
	}
	t.Logf("读数：yaml=%d 唯一id=%d 重复id组=%d 子目录=%d %v", nYaml, len(ids), len(dup), len(subdirs), subdirs)
	for _, d := range dup {
		// 内置目录里同 id＝有一份定义永远读不到（后加载者胜），这是回归闸门而不是提醒。
		t.Errorf("内置定义重复 id：%s", d)
	}
	if len(subdirs) > 0 {
		t.Errorf("内置 definitions 出现子目录 %v：加载器不递归，里面的定义会被整批跳过", subdirs)
	}

	// 正控：检测器（同一段逻辑）必须能抓出重复 id，否则上面的 0 不算证据。
	tmp := t.TempDir()
	a := []byte("id: samesite\nname: A\nschema: generic\n")
	b := []byte("id: samesite\nname: B\nschema: generic\n")
	if err := os.WriteFile(filepath.Join(tmp, "a.yaml"), a, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b.yaml"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.LoadDefinitions(tmp); err != nil {
		t.Fatalf("加载临时目录失败：%v", err)
	}
	got, ok := r.Definition("samesite")
	if !ok {
		t.Fatalf("正控前提不成立：同 id 的两个文件一个都没进注册表")
	}
	if got.Name != "B" {
		t.Errorf("正控读数不对：预期后者(name=B)顶掉前者，实际 Name=%q（说明覆盖方向与我描述的不同，先别下结论）", got.Name)
	} else {
		t.Logf("正控确认：同 id 时后加载的文件静默胜出，前者配置整个蒸发")
	}
}

// 接线护栏：两种「静默丢配置」都必须以 Warn 出声（级别连消息串一起钉，
// 只 Contains(消息串) 会放行 Debug 版实现）。
func TestLoaderWarnsOnSilentlyDroppedDefinitions(t *testing.T) {
	src, err := os.ReadFile("registry.go")
	if err != nil {
		t.Fatalf("读不到 registry.go：%v", err)
	}
	var keep []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		keep = append(keep, l)
	}
	c := strings.Join(keep, "\n")
	for _, want := range []string{
		`slog.Warn("site definitions subdirectory skipped`,
		`slog.Warn("site definition id duplicated within this directory`,
	} {
		if !strings.Contains(c, want) {
			t.Errorf("加载器必须对这一形出声（不否决加载，但要让人看见）：缺 %q", want)
		}
	}
	if strings.Contains(c, `slog.Debug("site definition`) || strings.Contains(c, `slog.Info("site definition id duplicated`) {
		t.Errorf("这两条是配置丢失，不许降级成 Debug/Info 当成正常读数")
	}
}
