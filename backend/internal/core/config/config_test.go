package config

import "testing"

// release 模式下未显式设密钥 → 自动生成 + 持久化,Load 成功（零配置安全启动）。
func TestLoadReleaseAutoGeneratesSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("SECRET_KEY", "")
	t.Setenv("ACTIVATION_SECRET", "")
	t.Setenv("DATA_DIR", t.TempDir()) // 隔离,避免污染 ./data
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to auto-generate secrets, got %v", err)
	}
	if cfg.SecretKey == "" || cfg.ActivationSecret == "" {
		t.Fatal("expected non-empty auto-generated secrets")
	}
	if cfg.SecretKey == cfg.ActivationSecret {
		t.Error("SECRET_KEY and ACTIVATION_SECRET should be independently generated (differ)")
	}
}

// release 模式下不得使用源码里的公开默认 SECRET_KEY。
func TestLoadReleaseRejectsDefaultSecret(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("SECRET_KEY", "dev-insecure-secret-change-me")
	t.Setenv("ACTIVATION_SECRET", "set-but-secret-is-default")
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to reject the public default SECRET_KEY in release mode")
	}
}

// release 模式下提供强密钥应正常启动。
func TestLoadReleaseAcceptsStrongSecrets(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("SECRET_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("ACTIVATION_SECRET", "activation-secret-must-be-set-in-release-mode")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed with strong secrets, got %v", err)
	}
	if !cfg.IsRelease() {
		t.Error("expected release mode")
	}
}

// debug 模式下空 SECRET_KEY 应回退到开发默认（不阻断开发）。
func TestLoadDebugUsesDevDefault(t *testing.T) {
	t.Setenv("APP_ENV", "debug")
	t.Setenv("SECRET_KEY", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("debug Load should not fail, got %v", err)
	}
	if cfg.SecretKey != "dev-insecure-secret-change-me" {
		t.Errorf("expected dev default secret, got %q", cfg.SecretKey)
	}
}
