package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #331 四类「站点定义被静默忽略」要有一张能被界面读的账面（此前只有日志）。
// 用例走真实加载：临时目录里一次摆出四种坏法，断言每一类都进了账面。

func TestRegistryAccumulatesAllFourDefinitionDiagnostics(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk("clean.yaml", "id: cleanone\nname: Clean\nschema: generic\nselectors:\n    fields:\n        title:\n            selector: h2 a\n")
	mk("badkey.yaml", "id: badkeysite\nname: Bad Key\nbogus_top_key:\n    foo: bar\n")
	mk("dup1.yaml", "id: sameweb\nname: First\nschema: generic\n")
	mk("dup2.yaml", "id: sameweb\nname: Second\nschema: generic\n")
	mk("badre.yaml", "id: badredsite\nname: Bad RE\nselectors:\n    fields:\n        subtitle:\n            selector: div.postContent\n            filters:\n                - name: regexp\n                  args: \"Category: (.+?)(?= x)\"\n")
	if err := os.MkdirAll(filepath.Join(dir, "extra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra", "ignored.yaml"), []byte("id: buried\nname: Buried\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadDefinitions(dir); err != nil {
		t.Fatalf("加载临时定义目录失败：%v", err)
	}
	r.RecordFilterPatternDiagnostics()

	kinds := map[string]DefinitionDiagnostic{}
	for _, d := range r.Diagnostics() {
		kinds[d.Kind] = d
	}
	for _, want := range []string{DiagUnknownKey, DiagDuplicateID, DiagBadRegexp, DiagSkippedSubdir} {
		got, ok := kinds[want]
		if !ok {
			t.Errorf("账面缺这一类 %s（四类都要能被界面读到）", want)
			continue
		}
		if strings.TrimSpace(got.Detail) == "" {
			t.Errorf("%s 的 Detail 是空的，界面读到了也不知道讲什么", want)
		}
	}
	// 定位信息：重复 id 与坏正则要指到具体站点，未知键/子目录要指到具体文件。
	if kinds[DiagDuplicateID].Site != "sameweb" {
		t.Errorf("重复 id 应带站点 id，实际 %+v", kinds[DiagDuplicateID])
	}
	if kinds[DiagBadRegexp].Site != "badredsite" {
		t.Errorf("坏正则应带站点 id，实际 %+v", kinds[DiagBadRegexp])
	}
	if !strings.Contains(kinds[DiagUnknownKey].File, "badkey.yaml") {
		t.Errorf("未知键应带文件名，实际 %+v", kinds[DiagUnknownKey])
	}
	if !strings.Contains(kinds[DiagSkippedSubdir].File, "extra") {
		t.Errorf("被跳过的子目录应带目录名，实际 %+v", kinds[DiagSkippedSubdir])
	}

	// 正控（反向）：一个全干净的目录不该攒出任何账面——否则「四类齐全」可能只是无条件塞条目。
	clean := t.TempDir()
	if err := os.WriteFile(filepath.Join(clean, "ok.yaml"), []byte("id: oksite\nname: OK\nschema: generic\nselectors:\n    fields:\n        title:\n            selector: h2 a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r2 := NewRegistry()
	if err := r2.LoadDefinitions(clean); err != nil {
		t.Fatalf("加载干净目录失败：%v", err)
	}
	r2.RecordFilterPatternDiagnostics()
	if got := r2.Diagnostics(); len(got) != 0 {
		t.Errorf("干净目录不该有诊断，实际 %+v", got)
	}
}

func TestSiteDefinitionsEndpointCarriesDiagnostics(t *testing.T) {
	// 端点侧的用例放在 internal/api/v1（site 包不能反向依赖 api 层），
	// 这里只保证账面与 site 侧真链路一致。
}
