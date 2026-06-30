package monitor

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"

	"golang.org/x/crypto/ssh"
)

// fakeHostKeyStore 是 HostKeyStore 的内存假实现（单测用，免真实 DB）。
type fakeHostKeyStore struct {
	keys     map[string][]byte
	recorded int
}

func (f *fakeHostKeyStore) Lookup(host string, port int) ([]byte, bool, error) {
	k, ok := f.keys[fmt.Sprintf("%s:%d", host, port)]
	return k, ok, nil
}

func (f *fakeHostKeyStore) Record(host string, port int, _ string, marshaled []byte, _ string) error {
	f.keys[fmt.Sprintf("%s:%d", host, port)] = marshaled
	f.recorded++
	return nil
}

func mustSSHKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	return k
}

// TestHostKeyCallbackTOFU 验证三态：首次记录、同 key 放行、不同 key 拒绝；以及 nil store 降级。
func TestHostKeyCallbackTOFU(t *testing.T) {
	k1 := mustSSHKey(t)
	k2 := mustSSHKey(t)
	st := &fakeHostKeyStore{keys: map[string][]byte{}}
	cb := makeHostKeyCallback(st, "host", 22)

	// 1. 首次连接：未记录 → 记录 + 放行。
	if err := cb("host", nil, k1); err != nil {
		t.Fatalf("first connect should record+accept, got %v", err)
	}
	if st.recorded != 1 {
		t.Fatalf("expected 1 record after first connect, got %d", st.recorded)
	}

	// 2. 同 key 再连：匹配 → 放行，不再记录。
	if err := cb("host", nil, k1); err != nil {
		t.Fatalf("second connect with same key should pass, got %v", err)
	}
	if st.recorded != 1 {
		t.Fatalf("expected still 1 record (match, no re-record), got %d", st.recorded)
	}

	// 3. 不同 key：不匹配 → 拒绝。
	if err := cb("host", nil, k2); err == nil {
		t.Fatal("expected mismatch error for different key, got nil")
	}

	// 4. nil store → 降级为不校验（兼容无注入）。
	nilCB := makeHostKeyCallback(nil, "host", 22)
	if err := nilCB("host", nil, k2); err != nil {
		t.Fatalf("nil store should bypass verification, got %v", err)
	}
}
