package oper

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func newMarketOper(t *testing.T) *PluginMarketOper {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/pm.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.PluginInstance{}); err != nil {
		t.Fatal(err)
	}
	return NewPluginMarketOper(db)
}

// 分身不存在时的「什么都没改」绝不能被报成「已保存」（#274 那一族：条件 UPDATE 不查 RowsAffected）。
func TestSaveInstanceConfigRejectsMissingInstance(t *testing.T) {
	o := newMarketOper(t)
	err := o.SaveInstanceConfig("nope", "main", `{}`, true)
	if err == nil {
		t.Fatal("不存在的分身应报错，实得 nil ⇒ 界面会显示「已保存」")
	}
	if !errors.Is(err, ErrPluginInstanceMissing) {
		t.Fatalf("应包 ErrPluginInstanceMissing 供 handler 分档，实得 %v", err)
	}

	// 正控+另一半：真的存在时同一段代码必须写成功（否则上面的红只是建表建歪了）。
	ins := &models.PluginInstance{PluginID: "douban", InstanceKey: "main", ConfigJSON: `{}`, Enabled: true}
	if err := o.db.Create(ins).Error; err != nil {
		t.Fatalf("造分身失败：%v", err)
	}
	if err := o.SaveInstanceConfig("douban", "main", `{"a":1}`, false); err != nil {
		t.Fatalf("存在的分身不该报错：%v", err)
	}
	got, err := o.GetInstance("douban", "main")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigJSON != `{"a":1}` || got.Enabled {
		t.Errorf("配置没写进去：%+v", got)
	}
}

// 排序提交了 N 条而库里只对得上 M 条时，不许回「已排序 N 条」。
func TestReorderInstancesRejectsUnknownIDs(t *testing.T) {
	o := newMarketOper(t)
	if err := o.ReorderInstances("douban", []uint{7, 8}); err == nil {
		t.Fatal("两条 id 都不存在却返回成功")
	} else if !errors.Is(err, ErrPluginInstanceMissing) {
		t.Errorf("应包 ErrPluginInstanceMissing，实得 %v", err)
	}
	// 一半存在一半不存在同样要出声（静默吞掉的那一条顺序并不会变）
	if err := o.db.Create(&models.PluginInstance{PluginID: "douban", InstanceKey: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	var id uint
	if err := o.db.Model(&models.PluginInstance{}).Where("instance_key = ?", "a").Select("id").Scan(&id).Error; err != nil {
		t.Fatal(err)
	}
	if err := o.ReorderInstances("douban", []uint{id, 999}); !errors.Is(err, ErrPluginInstanceMissing) {
		t.Errorf("只匹配到一半也该报错，实得 %v", err)
	}
	// 正控：全都能对上时必须成功
	if err := o.ReorderInstances("douban", []uint{id}); err != nil {
		t.Errorf("全部存在不该报错：%v", err)
	}
}
