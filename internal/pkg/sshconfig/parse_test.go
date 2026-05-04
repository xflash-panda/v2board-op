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

func TestParse_EqualsSignKeywordSyntax(t *testing.T) {
	path := writeTempConfig(t, `Host=server1
    HostName=1.2.3.4
    User=ubuntu
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
}

func TestParse_MultipleBlocks(t *testing.T) {
	path := writeTempConfig(t, `Host alpha
    HostName 1.1.1.1

Host beta
    HostName 2.2.2.2
    User root

Host gamma
    HostName 3.3.3.3
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d", len(cfg.Blocks))
	}
	wantAlias := []string{"alpha", "beta", "gamma"}
	wantIP := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
	for i, b := range cfg.Blocks {
		if b.Alias != wantAlias[i] || b.HostNameIP != wantIP[i] {
			t.Errorf("block %d: got alias=%q ip=%q, want %q %q",
				i, b.Alias, b.HostNameIP, wantAlias[i], wantIP[i])
		}
	}
}

func TestParse_SkipsWildcard(t *testing.T) {
	path := writeTempConfig(t, `Host *.example.com
    User root

Host real
    HostName 9.9.9.9
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(cfg.Blocks))
	}
	if !cfg.Blocks[0].Skip {
		t.Errorf("wildcard block should be Skip=true, reason=%q", cfg.Blocks[0].SkipReason)
	}
	if cfg.Blocks[1].Skip {
		t.Errorf("real block should not be skipped")
	}
}

func TestParse_SkipsMultiAlias(t *testing.T) {
	path := writeTempConfig(t, `Host a b c
    HostName 1.2.3.4
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(cfg.Blocks))
	}
	if !cfg.Blocks[0].Skip {
		t.Errorf("multi-alias block should be Skip=true")
	}
}

func TestParse_MatchBlockIgnored(t *testing.T) {
	path := writeTempConfig(t, `Host real
    HostName 1.1.1.1

Match host other
    User x
    HostName 2.2.2.2

Host another
    HostName 3.3.3.3
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Expect: real, another. The Match block produces no HostBlock.
	if len(cfg.Blocks) != 2 {
		t.Fatalf("want 2 blocks (real, another), got %d", len(cfg.Blocks))
	}
	if cfg.Blocks[0].Alias != "real" || cfg.Blocks[1].Alias != "another" {
		t.Errorf("got aliases %q,%q want real,another",
			cfg.Blocks[0].Alias, cfg.Blocks[1].Alias)
	}
}

func TestParse_BlockWithoutHostName(t *testing.T) {
	path := writeTempConfig(t, `Host orphan
    User x
    Port 22
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(cfg.Blocks))
	}
	if cfg.Blocks[0].HostNameLine != -1 || cfg.Blocks[0].HostNameIP != "" {
		t.Errorf("orphan block should have no HostName: line=%d ip=%q",
			cfg.Blocks[0].HostNameLine, cfg.Blocks[0].HostNameIP)
	}
}

func TestParse_IndentedHostNameLineNumber(t *testing.T) {
	// 0: Host alpha
	// 1: (blank)
	// 2:     HostName 1.1.1.1
	path := writeTempConfig(t, "Host alpha\n\n    HostName 1.1.1.1\n")
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cfg.Blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(cfg.Blocks))
	}
	if cfg.Blocks[0].HostNameLine != 2 {
		t.Errorf("hostname line=%d want 2", cfg.Blocks[0].HostNameLine)
	}
}

func TestParse_FirstHostNameWins(t *testing.T) {
	path := writeTempConfig(t, `Host dup
    HostName 1.1.1.1
    HostName 2.2.2.2
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Blocks[0].HostNameIP != "1.1.1.1" {
		t.Errorf("first HostName should win, got %q", cfg.Blocks[0].HostNameIP)
	}
}
