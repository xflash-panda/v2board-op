package sshconfig

import (
	"sort"
	"testing"
)

func TestDiff_DetectsChanges(t *testing.T) {
	cfg := &Config{
		Lines: []string{},
		Blocks: []*HostBlock{
			{Alias: "alpha", HostNameLine: 1, HostNameIP: "1.1.1.1"},
			{Alias: "beta", HostNameLine: 4, HostNameIP: "2.2.2.2"},
			{Alias: "gamma", HostNameLine: 7, HostNameIP: "3.3.3.3"},
		},
	}
	instances := map[string]string{
		"alpha": "1.1.1.99", // changed
		"beta":  "2.2.2.2",  // unchanged
		// gamma missing
	}
	updates, unchanged, unmatched := cfg.Diff(instances)

	if len(updates) != 1 || updates[0].Alias != "alpha" || updates[0].NewIP != "1.1.1.99" {
		t.Errorf("updates wrong: %+v", updates)
	}
	if unchanged != 1 {
		t.Errorf("unchanged=%d want 1", unchanged)
	}
	sort.Strings(unmatched)
	if len(unmatched) != 1 || unmatched[0] != "gamma" {
		t.Errorf("unmatched=%v want [gamma]", unmatched)
	}
}

func TestDiff_SkipsSkippedBlocks(t *testing.T) {
	cfg := &Config{
		Blocks: []*HostBlock{
			{Alias: "*.example.com", Skip: true, SkipReason: "wildcard"},
		},
	}
	instances := map[string]string{"alpha": "1.1.1.1"}
	updates, _, unmatched := cfg.Diff(instances)
	if len(updates) != 0 {
		t.Errorf("skipped block should not produce update, got %+v", updates)
	}
	if len(unmatched) != 0 {
		t.Errorf("skipped block should not appear in unmatched, got %v", unmatched)
	}
}

func TestDiff_BlockWithoutHostNameIgnored(t *testing.T) {
	cfg := &Config{
		Blocks: []*HostBlock{
			{Alias: "orphan", HostNameLine: -1},
		},
	}
	instances := map[string]string{"orphan": "9.9.9.9"}
	updates, _, _ := cfg.Diff(instances)
	if len(updates) != 0 {
		t.Errorf("block without HostName should not be updated, got %+v", updates)
	}
}

func TestRenderDiff_Format(t *testing.T) {
	updates := []Update{
		{Alias: "alpha", OldIP: "1.1.1.1", NewIP: "9.9.9.9", Line: 1},
		{Alias: "beta", OldIP: "2.2.2.2", NewIP: "8.8.8.8", Line: 4},
	}
	out := RenderDiff(updates, 3, []string{"gamma", "delta"})

	want := `Host alpha
- HostName 1.1.1.1
+ HostName 9.9.9.9

Host beta
- HostName 2.2.2.2
+ HostName 8.8.8.8

2 hosts to update, 3 unchanged, 2 unmatched (skipped).
`
	if out != want {
		t.Errorf("RenderDiff output mismatch.\nGOT:\n%s\nWANT:\n%s", out, want)
	}
}

func TestRenderDiff_NoUpdates(t *testing.T) {
	out := RenderDiff(nil, 5, nil)
	want := "0 hosts to update, 5 unchanged, 0 unmatched (skipped).\n"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
