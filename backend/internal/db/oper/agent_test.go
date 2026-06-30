package oper

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newAgentTxDB 内存级测试库，迁移 Agent + MonitorTarget 两张表。
func newAgentTxDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent_tx.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MonitorTarget{}, &models.Agent{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestAgentCreateTransactionAtomicity 验证 WithTx 参与事务：
// 事务里先建 target 再故意失败 → 回滚后 target 不应残留（杜绝孤儿监控目标）。
func TestAgentCreateTransactionAtomicity(t *testing.T) {
	db := newAgentTxDB(t)
	monOp := NewMonitorOper(db, security.NewCrypto("test-seed"))
	_ = NewAgentOper(db) // 占位：本用例只验证 target 回滚

	err := db.Transaction(func(tx *gorm.DB) error {
		if _, e := monOp.WithTx(tx).Create(TargetInput{
			Name: "orphan-candidate", Type: "host", Protocol: "agent", Enabled: true,
			IntervalSec: 5, TimeoutSec: 15,
		}); e != nil {
			return e
		}
		return errors.New("simulated agent create failure") // 模拟第二次写失败
	})
	if err == nil {
		t.Fatal("expected transaction to fail")
	}

	var n int64
	db.Model(&models.MonitorTarget{}).Count(&n)
	if n != 0 {
		t.Errorf("transaction did not roll back: %d MonitorTarget rows leaked", n)
	}

	// 对照：非事务直接创建应成功，证明上面是事务在起作用而非 Create 本身失败
	if _, err := monOp.Create(TargetInput{Name: "ok", Type: "host", Protocol: "agent", Enabled: true}); err != nil {
		t.Fatalf("non-tx create should succeed: %v", err)
	}
}

// TestAgentCreateTransactionCommit 验证成功路径：target + agent 均落库。
func TestAgentCreateTransactionCommit(t *testing.T) {
	db := newAgentTxDB(t)
	monOp := NewMonitorOper(db, security.NewCrypto("test-seed"))
	agentOp := NewAgentOper(db)

	var agentID uint
	err := db.Transaction(func(tx *gorm.DB) error {
		tgt, e := monOp.WithTx(tx).Create(TargetInput{
			Name: "host1", Type: "host", Protocol: "agent", Enabled: true,
		})
		if e != nil {
			return e
		}
		r, e := agentOp.WithTx(tx).Create("agent1", tgt.ID)
		if e != nil {
			return e
		}
		agentID = r.Agent.ID
		return nil
	})
	if err != nil {
		t.Fatalf("transaction should succeed: %v", err)
	}

	var tn, an int64
	db.Model(&models.MonitorTarget{}).Count(&tn)
	db.Model(&models.Agent{}).Count(&an)
	if tn != 1 || an != 1 {
		t.Errorf("expected 1 target + 1 agent, got target=%d agent=%d", tn, an)
	}
	if agentID == 0 {
		t.Error("agent id not captured from transaction")
	}
}
