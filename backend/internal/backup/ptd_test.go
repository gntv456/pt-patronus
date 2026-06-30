package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// makePTDZip 把 map[文件名]内容 打成 ZIP 字节（模拟 .ptd）。
func makePTDZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newPTDSvc(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ptd.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	c := security.NewCrypto("test-seed-for-ptd-unit-tests-32bytes!")
	return New(oper.NewSiteOper(db, c), nil)
}

const ptdManifest = `{"encryption":false,"version":"PT-Depiler","files":{"metadata":{"name":"metadata.json"},"cookies":{"name":"cookies.json"}}}`

const ptdMetadata = `{
  "sites": {
    "site_1": {
      "url": "https://www.example-pt.org",
      "merge": {"name":"示例站","schema":"NexusPHP"},
      "inputSetting": {"username":"alice","password":"pw123","passkey":"PK123"},
      "sortIndex": 1
    },
    "mteam": {
      "url": "kp.m-team.cc",
      "merge": {"schema":"MTeam"},
      "inputSetting": {"passkey":"MTPK"},
      "isOffline": true
    }
  },
  "siteNameMap": {"site_1":"示例别名"},
  "lastUserInfo": {"site_1":{"name":"alice_real"}},
  "siteHostMap": {"www.example-pt.org":"site_1","kp.m-team.cc":"mteam"}
}`

const ptdCookies = `{
  "www.example-pt.org":[{"name":"c1","value":"v1"},{"name":"c2","value":"v2"}],
  "kp.m-team.cc":[{"name":"mt","value":"mtv"}]
}`

func TestImportPTD(t *testing.T) {
	svc := newPTDSvc(t)
	data := makePTDZip(t, map[string]string{
		"manifest.json": ptdManifest,
		"metadata.json": ptdMetadata,
		"cookies.json":  ptdCookies,
	})
	n, err := svc.ImportPTD(context.Background(), data)
	if err != nil {
		t.Fatalf("ImportPTD: %v", err)
	}
	if n != 2 {
		t.Fatalf("imported %d sites, want 2", n)
	}

	sites, err := svc.siteOp.List()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]models.Site{}
	for _, s := range sites {
		byName[s.Name] = s
	}

	// site_1：name 取 siteNameMap（优先于 merge.name）、username 取 lastUserInfo（优先于 inputSetting）。
	s1, ok := byName["示例别名"]
	if !ok {
		t.Fatalf("site 示例别名 not imported; got %v", byNameKeys(byName))
	}
	if s1.BaseURL != "https://www.example-pt.org" {
		t.Errorf("BaseURL=%q", s1.BaseURL)
	}
	if s1.Platform != "nexusphp" {
		t.Errorf("Platform=%q, want nexusphp", s1.Platform)
	}
	if s1.Username != "alice_real" {
		t.Errorf("Username=%q, want alice_real", s1.Username)
	}
	if !s1.IsActive {
		t.Errorf("IsActive=false, want true")
	}
	if pk := svc.siteOp.DecryptedPasskey(&s1); pk != "PK123" {
		t.Errorf("Passkey=%q, want PK123", pk)
	}
	if pw, _ := svc.siteOp.Password(&s1); pw != "pw123" {
		t.Errorf("Password=%q, want pw123", pw)
	}
	cookie, _ := svc.siteOp.Cookie(&s1)
	if !strings.Contains(cookie, "c1=v1") || !strings.Contains(cookie, "c2=v2") {
		t.Errorf("Cookie=%q, want both c1=v1 and c2=v2", cookie)
	}

	// mteam：platform 按 id 推断、isOffline→不启用、无 scheme 补 https://、name 回退 id。
	mt, ok := byName["mteam"]
	if !ok {
		t.Fatalf("site mteam not imported")
	}
	if mt.Platform != "mteam" {
		t.Errorf("mteam Platform=%q, want mteam", mt.Platform)
	}
	if mt.IsActive {
		t.Errorf("mteam IsActive=true, want false (isOffline)")
	}
	if mt.BaseURL != "https://kp.m-team.cc" {
		t.Errorf("mteam BaseURL=%q", mt.BaseURL)
	}
	if ck, _ := svc.siteOp.Cookie(&mt); ck != "mt=mtv" {
		t.Errorf("mteam Cookie=%q, want mt=mtv", ck)
	}
}

func TestImportPTDEncryptedRejected(t *testing.T) {
	svc := newPTDSvc(t)
	data := makePTDZip(t, map[string]string{
		"manifest.json": `{"encryption":true,"version":"PT-Depiler"}`,
	})
	_, err := svc.ImportPTD(context.Background(), data)
	if err == nil || !strings.Contains(err.Error(), "加密") {
		t.Fatalf("want encryption rejection, got %v", err)
	}
}

func TestImportPTDNoManifestRejected(t *testing.T) {
	svc := newPTDSvc(t)
	data := makePTDZip(t, map[string]string{"metadata.json": ptdMetadata})
	_, err := svc.ImportPTD(context.Background(), data)
	if err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("want non-PTD rejection, got %v", err)
	}
}

func TestImportPTDSkipsSiteWithoutURL(t *testing.T) {
	svc := newPTDSvc(t)
	meta := `{"sites":{"bad":{"merge":{"name":"无URL站"}},"good":{"url":"https://good.org"}}}`
	data := makePTDZip(t, map[string]string{"manifest.json": ptdManifest, "metadata.json": meta})
	n, err := svc.ImportPTD(context.Background(), data)
	if err != nil {
		t.Fatalf("ImportPTD: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported %d, want 1 (bad site without URL skipped)", n)
	}
}

func byNameKeys(m map[string]models.Site) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
