package oper

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

// newBindingValidator 构造一个读 "binding" tag 的 validator（模拟 gin ShouldBindJSON 行为）。
// gin 内部 binding 包用 validator 且 tag key = "binding"，这里复刻以直接验证 oper struct 的 tag。
func newBindingValidator(t *testing.T) *validator.Validate {
	t.Helper()
	v := validator.New()
	v.SetTagName("binding")
	return v
}

// TestCredentialInputBindingMax 验证 CredentialInput 各字段 binding max 生效：正常通过、超长被拒。
func TestCredentialInputBindingMax(t *testing.T) {
	v := newBindingValidator(t)
	if err := v.Struct(CredentialInput{Name: "服务器A", Type: "ssh", Tags: "prod"}); err != nil {
		t.Fatalf("normal input should pass: %v", err)
	}
	cases := []struct {
		name string
		in   CredentialInput
	}{
		{"Name>64", CredentialInput{Name: strings.Repeat("a", 65)}},
		{"Type>16", CredentialInput{Type: strings.Repeat("a", 17)}},
		{"Note>2000", CredentialInput{Note: strings.Repeat("a", 2001)}},
		{"PrivateKey>8192", CredentialInput{PrivateKey: strings.Repeat("a", 8193)}},
	}
	for _, tc := range cases {
		if err := v.Struct(tc.in); err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
	}
}

// TestTargetInputBindingMax 验证 TargetInput 含特殊大值字段：PrivateKey(8192)/Thresholds(5000)/DiskPaths(2000)。
// 正常长度（含 4KB PEM 私钥）应通过，超限被拒。
func TestTargetInputBindingMax(t *testing.T) {
	v := newBindingValidator(t)
	if err := v.Struct(TargetInput{Name: "监控A", Type: "host", Host: "127.0.0.1"}); err != nil {
		t.Fatalf("normal input should pass: %v", err)
	}
	if err := v.Struct(TargetInput{Name: "n", PrivateKey: strings.Repeat("a", 4000)}); err != nil {
		t.Errorf("normal-length PrivateKey (4KB) should pass: %v", err)
	}
	cases := []struct {
		name string
		in   TargetInput
	}{
		{"PrivateKey>8192", TargetInput{PrivateKey: strings.Repeat("a", 8193)}},
		{"Thresholds>5000", TargetInput{Thresholds: strings.Repeat("a", 5001)}},
		{"DiskPaths>2000", TargetInput{DiskPaths: strings.Repeat("a", 2001)}},
		{"Host>255", TargetInput{Host: strings.Repeat("a", 256)}},
	}
	for _, tc := range cases {
		if err := v.Struct(tc.in); err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
	}
}
