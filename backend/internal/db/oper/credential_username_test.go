// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

func credOper(t *testing.T, db *gorm.DB, seed string) *CredentialOper {
	t.Helper()
	return NewCredentialOper(db, security.NewCrypto(seed))
}

// TestCredentialUsernameDistinguishesMissingFromUnreadable 回归：fill 过去把「解密失败」和
// 「没填用户名」都留成 Username=""，而同一行的 has_password/has_private_key 只看密文非空、
// 依旧为真 —— 主密钥换过后列表显示成「无用户名 · 密码 · 私钥」这种自相矛盾的行，
// 用户会以为凭据坏了而删掉重建。
func TestCredentialUsernameDistinguishesMissingFromUnreadable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cred.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.OpsCredential{}, &models.MonitorTarget{}); err != nil {
		t.Fatal(err)
	}

	created, err := credOper(t, db, "original-key").Create(CredentialInput{
		Name: "服务器A", Type: "ssh", Username: "root", Password: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasUsername || created.Username != "root" {
		t.Fatalf("同一把密钥下要能读回用户名，got %+v", created)
	}

	rotated, err := credOper(t, db, "rotated-key").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rotated) != 1 {
		t.Fatalf("应有 1 条凭据，got %d", len(rotated))
	}
	c := rotated[0]
	if c.Username != "" {
		t.Errorf("解不开时不该给出（可能是别的密钥解出的）用户名，got %q", c.Username)
	}
	if !c.HasUsername {
		t.Error("HasUsername 必须为真：存过用户名，只是现在解不开，不是「没填」")
	}
	if !c.HasPassword {
		t.Error("has_password 口径不受影响，仍应为真")
	}

	// 对照组：真没填用户名的凭据不能被说成「解不开」
	if _, err := credOper(t, db, "rotated-key").Create(CredentialInput{
		Name: "只填密码", Type: "generic", Password: "pw",
	}); err != nil {
		t.Fatal(err)
	}
	all, err := credOper(t, db, "rotated-key").List()
	if err != nil {
		t.Fatal(err)
	}
	var bare *models.OpsCredential
	for i := range all {
		if all[i].Name == "只填密码" {
			bare = &all[i]
		}
	}
	if bare == nil {
		t.Fatal("没找到对照凭据")
	}
	if bare.HasUsername {
		t.Error("真没填用户名的凭据 HasUsername 必须为假")
	}
}
