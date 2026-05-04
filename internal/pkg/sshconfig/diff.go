package sshconfig

// Update describes one HostName line that should be replaced.
type Update struct {
	Alias string
	OldIP string
	NewIP string
	Line  int
}

// Diff compares the parsed Config against a name->IP map and returns:
//   - updates: HostName lines whose IP differs from the instance map
//   - unchanged: count of blocks whose IP is already correct
//   - unmatched: aliases present in the config but absent from the instance map
//
// Skipped blocks (wildcards, multi-alias, Match) and blocks without a
// HostName line are ignored entirely — never updated, never reported.
func (c *Config) Diff(instances map[string]string) (updates []Update, unchanged int, unmatched []string) {
	for _, b := range c.Blocks {
		if b.Skip || b.HostNameLine == -1 {
			continue
		}
		newIP, ok := instances[b.Alias]
		if !ok {
			unmatched = append(unmatched, b.Alias)
			continue
		}
		if newIP == b.HostNameIP {
			unchanged++
			continue
		}
		updates = append(updates, Update{
			Alias: b.Alias,
			OldIP: b.HostNameIP,
			NewIP: newIP,
			Line:  b.HostNameLine,
		})
	}
	return updates, unchanged, unmatched
}
