package automation

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fakeKeepSeed 记录 Reseed/Resolve 调用；ListAutoReseedSettings 返回预设设置，Reseed 返回 2 个 pending 候选。
type fakeKeepSeed struct {
	settings []models.CrossSeedSetting
	reseeded []uint // 记录被 Reseed 的 downloader_id
	added    []uint // 记录被 Resolve(add) 的 task id
}

func (f *fakeKeepSeed) ListAutoReseedSettings() ([]models.CrossSeedSetting, error) {
	return f.settings, nil
}
func (f *fakeKeepSeed) Reseed(_ context.Context, dl uint) ([]models.CrossSeedTask, error) {
	f.reseeded = append(f.reseeded, dl)
	return []models.CrossSeedTask{
		{ID: 1, DownloaderID: dl, Status: "pending"},
		{ID: 2, DownloaderID: dl, Status: "pending"},
	}, nil
}
func (f *fakeKeepSeed) Resolve(_ context.Context, taskID uint, action string) error {
	if action == "add" {
		f.added = append(f.added, taskID)
	}
	return nil
}

// newSvc 用 fake keepSeed 造 Service（其余依赖 nil——runKeepSeed 不用它们）。
func newSvc(fk *fakeKeepSeed) *Service {
	return New(nil, nil, nil, nil, nil, fk, nil, Config{}, context.Background(), nil, nil, nil, nil)
}

func TestRunKeepSeed(t *testing.T) {
	// ① scan-only：AutoReseedAdd=false → 只 Reseed 产 pending，不 add
	fk := &fakeKeepSeed{settings: []models.CrossSeedSetting{{DownloaderID: 1, AutoReseed: true, AutoReseedAdd: false}}}
	newSvc(fk).runKeepSeed()
	if len(fk.reseeded) != 1 || fk.reseeded[0] != 1 {
		t.Errorf("scan-only: reseeded = %v, want [1]", fk.reseeded)
	}
	if len(fk.added) != 0 {
		t.Errorf("scan-only: added = %v, want none", fk.added)
	}

	// ② auto-add：AutoReseedAdd=true → created 全部 Resolve(add)
	fk2 := &fakeKeepSeed{settings: []models.CrossSeedSetting{{DownloaderID: 2, AutoReseed: true, AutoReseedAdd: true}}}
	newSvc(fk2).runKeepSeed()
	if len(fk2.reseeded) != 1 || fk2.reseeded[0] != 2 {
		t.Errorf("auto-add: reseeded = %v, want [2]", fk2.reseeded)
	}
	if len(fk2.added) != 2 {
		t.Errorf("auto-add: added = %v, want 2 tasks", fk2.added)
	}

	// ③ 无下载器启用 AutoReseed → 零动作
	fk3 := &fakeKeepSeed{settings: nil}
	newSvc(fk3).runKeepSeed()
	if len(fk3.reseeded) != 0 {
		t.Errorf("no-enabled: reseeded = %v, want none", fk3.reseeded)
	}
}
