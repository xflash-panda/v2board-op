package sshconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestParse_SingleHostBlock(t *testing.T) {
	path := writeTempConfig(t, `Host server1
    HostName 1.2.3.4
    User ubuntu
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(cfg.Blocks))
	}
	b := cfg.Blocks[0]
	if b.Alias != "server1" {
		t.Errorf("alias=%q want server1", b.Alias)
	}
	if b.HostNameIP != "1.2.3.4" {
		t.Errorf("ip=%q want 1.2.3.4", b.HostNameIP)
	}
	if b.HostNameLine != 1 {
		t.Errorf("hostname line=%d want 1", b.HostNameLine)
	}
	if b.Skip {
		t.Errorf("should not be skipped")
	}
}
