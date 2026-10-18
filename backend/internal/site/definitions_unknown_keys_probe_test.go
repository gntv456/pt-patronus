package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #329 站点定义 YAML 的未知键：默认 yaml.Unmarshal 不报错不出声，整块配置静默蒸发。
// 判据两半：①现网 306 个内置定义必须一个未知键都没有（这条是回归护栏——将来有人打错键就红）；
// ②检测函数本身必须抓得到坏键，且**不否决加载**（一个键错不该让外部目录整批退回内置）。

func TestNoUnknownKeysInBundledDefinitions(t *testing.T) {
	dir := filepath.Join("..", "..", "definitions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读不到 %s：%v", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) < 100 {
		t.Fatalf("基数只有 %d 个文件（下面的「0 命中」不算证据）", len(files))
	}

	// 正控：判据自己必须先能抓出一条坏键。
	if ctl := DefinitionUnknownKeys([]byte("id: ctl\nbogus_top_key:\n  - 1\n")); len(ctl) != 1 {
		t.Fatalf("正控失效：注入 bogus_top_key 应抓到 1 条，实际 %d 条（那 0 命中没有意义）", len(ctl))
	} else {
		t.Logf("正控命中：%q", ctl[0])
	}

	hits := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		if bad := DefinitionUnknownKeys(data); len(bad) > 0 {
			hits++
			t.Errorf("%s 有被加载器忽略的键：%q", filepath.Base(f), bad)
		}
	}
	t.Logf("扫描 %d 个定义文件，含未知键的文件数=%d", len(files), hits)
}

func TestUnknownKeyDoesNotBlockLoading(t *testing.T) {
	dir := t.TempDir()
	bad := []byte("id: probebadkey\nname: Probe Bad Key\nbogus_top_key:\n    foo: bar\nselectors:\n    fields:\n        title:\n            selector: h2 a\n")
	if err := os.WriteFile(filepath.Join(dir, "probebadkey.yaml"), bad, 0o600); err != nil {
		t.Fatalf("写探针文件失败：%v", err)
	}
	r := NewRegistry()
	if err := r.LoadDefinitions(dir); err != nil {
		t.Errorf("未知键只许出声、不许否决加载（否则用户一个笔误就丢掉整个自定义目录）：%v", err)
	}
	def, ok := r.Definition("probebadkey")
	if !ok {
		t.Fatalf("该定义应仍在注册表里（加载没被打断）")
	}
	if len(def.Selectors.Fields) != 1 {
		t.Errorf("已知键的部分应照常生效，实际字段数=%d", len(def.Selectors.Fields))
	}
	if got := DefinitionUnknownKeys(bad); len(got) != 1 {
		t.Errorf("这条坏键必须被检测出来（加载照常≠不吭声），实际 %q", got)
	}
	// 干净文件不许误报。
	if clean := DefinitionUnknownKeys([]byte("id: plain\nname: Plain\nselectors:\n    fields:\n        title:\n            selector: h2 a\n")); len(clean) != 0 {
		t.Errorf("合法定义不该被报未知键：%q", clean)
	}
}

// 接线护栏：加载器必须真的调用检测并 Warn（级别连串一起钉，只 Contains(函数名) 会放行「扫了但不出声」）。
func TestLoaderWiresUnknownKeyWarning(t *testing.T) {
	reg := readSiteSource(t, "registry.go")
	i := strings.Index(reg, "if err := yaml.Unmarshal(data, &def); err != nil {")
	if i < 0 {
		t.Fatalf("找不到 registry.go 的定义解析处")
	}
	window := reg[i:min(i+400, len(reg))]
	if !strings.Contains(window, "warnUnknownDefinitionKeys(file, data)") {
		t.Errorf("解析成功后必须跑未知键检测：%q", strings.TrimSpace(window))
	}
	if strings.Index(window, "warnUnknownDefinitionKeys") < strings.Index(window, "return fmt.Errorf(\"parse") {
		// 语法错误那条 return 在前是允许的（解析失败谈不上未知键），这里只确认调用点在 Unmarshal 之后。
		t.Log("注：warnUnknownDefinitionKeys 位于 parse 错误分支之后（预期）")
	}

	lint := readSiteSource(t, "definitions_lint.go")
	if !strings.Contains(lint, `slog.Warn("site definition has a key that is ignored`) {
		t.Errorf("检测必须 Warn 出声（Debug 级默认看不见）")
	}
	for _, k := range []string{`"file"`, `"detail"`} {
		if !strings.Contains(lint, k) {
			t.Errorf("留痕要带 %s 才找得到是哪个文件哪个键：%q", k, k)
		}
	}
}

func readSiteSource(t *testing.T, file string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读不到 %s：%v", file, err)
	}
	var out []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
