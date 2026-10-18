package oper

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func setupRuleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openIsolatedDB(t) // 每条用例一个命名内存库，避免同进程共享缓存串数据（见 testdb_test.go）
	if err := db.AutoMigrate(&models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// 新字段（output_tag/include_exts/exclude_keywords/双地址）建-读-改全链路不丢。
func TestStrmRuleNewFieldsRoundTrip(t *testing.T) {
	op := NewStrmRuleOper(setupRuleDB(t))
	r, err := op.Create(StrmRuleInput{
		Name: "电影", DriveID: 1, CloudPath: "/电影", OutputDir: "/strm",
		OutputTag: "strm-movie", IncludeExts: "mkv,mp4", ExcludeKeywords: "sample,trailer",
		LocalBaseURL: "http://192.168.1.10:8080", PublicBaseURL: "https://ms.example.com",
		Recursive: boolPtr(true), KeepExt: boolPtr(true), Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.OutputTag != "strm-movie" || r.IncludeExts != "mkv,mp4" || r.ExcludeKeywords != "sample,trailer" ||
		r.LocalBaseURL != "http://192.168.1.10:8080" || r.PublicBaseURL != "https://ms.example.com" {
		t.Fatalf("create lost fields: %+v", r)
	}
	// 更新：新字段总是覆盖（空串=清空）
	if _, err := op.Update(r.ID, StrmRuleInput{Name: "电影2", DriveID: 1, CloudPath: "/电影", OutputDir: "/strm"}); err != nil {
		t.Fatal(err)
	}
	got, _ := op.Get(r.ID)
	if got.Name != "电影2" || got.OutputTag != "" || got.IncludeExts != "" {
		t.Fatalf("update should overwrite new fields to empty: %+v", got)
	}
}

func boolPtr(b bool) *bool { return &b }
