package change_ip

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/xflash-panda/v2board-op/internal/pkg/api"
)

func TestLenStaticIps_Empty(t *testing.T) {
	job := &LightsailCFJob{}
	if got := job.lenStaticIps(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestLenStaticIps_CountsCorrectly(t *testing.T) {
	job := &LightsailCFJob{}
	job.staticIps.Store("1.1.1.1", types.StaticIp{})
	job.staticIps.Store("2.2.2.2", types.StaticIp{})
	job.staticIps.Store("3.3.3.3", types.StaticIp{})

	if got := job.lenStaticIps(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestStaticIpsCacheDelete(t *testing.T) {
	job := &LightsailCFJob{}
	for i := 0; i < 5; i++ {
		ip := "1.1.1." + string(rune('1'+i))
		job.staticIps.Store(ip, types.StaticIp{})
	}

	if got := job.lenStaticIps(); got != 5 {
		t.Fatalf("expected 5 before delete, got %d", got)
	}

	job.staticIps.Delete("1.1.1.1")

	if got := job.lenStaticIps(); got != 4 {
		t.Errorf("expected 4 after delete, got %d", got)
	}
}

func TestStaticIpsCacheStoreAfterAllocate(t *testing.T) {
	job := &LightsailCFJob{}
	job.staticIps.Store("1.1.1.1", types.StaticIp{})
	job.staticIps.Store("2.2.2.2", types.StaticIp{})

	if got := job.lenStaticIps(); got != 2 {
		t.Fatalf("expected 2 before allocate, got %d", got)
	}

	job.staticIps.Store("3.3.3.3", types.StaticIp{})

	if got := job.lenStaticIps(); got != 3 {
		t.Errorf("expected 3 after allocate, got %d", got)
	}
}

func TestProcessInstanceRouting_MaxStaticIps_MustRestart(t *testing.T) {
	job := &LightsailCFJob{}
	for i := 0; i < 5; i++ {
		ip := "10.0.0." + string(rune('1'+i))
		job.staticIps.Store(ip, types.StaticIp{})
	}

	if got := job.lenStaticIps(); got < 5 {
		t.Fatalf("expected >= 5 static IPs, got %d", got)
	}
}

func TestProcessInstanceRouting_BelowMax_Allocate(t *testing.T) {
	job := &LightsailCFJob{}
	job.staticIps.Store("10.0.0.1", types.StaticIp{})

	if job.lenStaticIps() >= 5 {
		t.Error("should NOT trigger restart path when below max")
	}
}

func TestProcessInstanceRouting_ReleaseFreesSlot(t *testing.T) {
	job := &LightsailCFJob{}
	for i := 0; i < 5; i++ {
		ip := "10.0.0." + string(rune('1'+i))
		job.staticIps.Store(ip, types.StaticIp{})
	}

	if job.lenStaticIps() < 5 {
		t.Fatal("precondition: expected 5 static IPs")
	}

	job.staticIps.Delete("10.0.0.1")

	if job.lenStaticIps() >= 5 {
		t.Error("after releasing one static IP, count should be below max, allowing allocate path")
	}
}

func TestConfigCheck_EmptyTags(t *testing.T) {
	conf := &LightsailCFJobConfig{}
	if err := conf.check(); err == nil {
		t.Error("expected error for empty tags")
	}
}

func TestConfigCheck_Valid(t *testing.T) {
	conf := &LightsailCFJobConfig{QueryTags: []string{"tag1"}}
	if err := conf.check(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestUniqueHosts_DedupesAcrossItems(t *testing.T) {
	items := []*api.BannedHostInfo{
		{Host: "trojan.example.com", IP: "1.1.1.1"},
		{Host: "anytls.example.com", IP: "1.1.1.2"},
		{Host: "trojan.example.com", IP: "1.1.1.3"},
	}
	hosts := uniqueHosts(items)
	if len(hosts) != 2 {
		t.Fatalf("expected 2 unique hosts, got %d (%v)", len(hosts), hosts)
	}
	got := map[string]bool{hosts[0]: true, hosts[1]: true}
	if !got["trojan.example.com"] || !got["anytls.example.com"] {
		t.Errorf("missing expected host: %v", hosts)
	}
}

func TestUniqueHosts_SkipsItemsWithoutHost(t *testing.T) {
	items := []*api.BannedHostInfo{
		{Host: "", IP: "1.1.1.1"},
		{Host: "trojan.example.com", IP: "1.1.1.2"},
	}
	hosts := uniqueHosts(items)
	if len(hosts) != 1 || hosts[0] != "trojan.example.com" {
		t.Errorf("expected only the item with host: %v", hosts)
	}
}

func TestDnsKey_Composition(t *testing.T) {
	if got := dnsKey("a.example.com", "1.1.1.1"); got != "a.example.com|1.1.1.1" {
		t.Errorf("unexpected dnsKey: %s", got)
	}
}

func TestLockFilePath_StableAcrossOrdering(t *testing.T) {
	a := lockFilePath(&LightsailCFJobConfig{QueryTags: []string{"tag1", "tag2"}})
	b := lockFilePath(&LightsailCFJobConfig{QueryTags: []string{"tag2", "tag1"}})
	if a != b {
		t.Errorf("lock path should be stable across tag ordering: %s vs %s", a, b)
	}
}

func TestLockFilePath_DiffersForDifferentTags(t *testing.T) {
	a := lockFilePath(&LightsailCFJobConfig{QueryTags: []string{"tag1"}})
	b := lockFilePath(&LightsailCFJobConfig{QueryTags: []string{"tag2"}})
	if a == b {
		t.Errorf("different tags should produce different lock paths: %s", a)
	}
}

func TestConfigCheck_ZeroConcurrency_DefaultsValid(t *testing.T) {
	conf := &LightsailCFJobConfig{
		QueryTags:   []string{"tag1"},
		Concurrency: 0,
	}
	if err := conf.check(); err != nil {
		t.Errorf("zero concurrency should be valid (will default at runtime), got: %v", err)
	}
}

func TestConfigCheck_NegativeConcurrency(t *testing.T) {
	conf := &LightsailCFJobConfig{
		QueryTags:   []string{"tag1"},
		Concurrency: -1,
	}
	if err := conf.check(); err == nil {
		t.Error("expected error for negative concurrency")
	}
}

func TestStatsAtomicSafety(t *testing.T) {
	stats := &LightsailJobStats{}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats.success.Add(1)
			stats.fail.Add(1)
		}()
	}
	wg.Wait()

	if got := stats.success.Load(); got != 100 {
		t.Errorf("expected success=100, got %d", got)
	}
	if got := stats.fail.Load(); got != 100 {
		t.Errorf("expected fail=100, got %d", got)
	}
}

func TestStaticIpMutexPreventsOverAllocation(t *testing.T) {
	job := &LightsailCFJob{}
	for i := 0; i < 4; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		job.staticIps.Store(ip, types.StaticIp{})
	}

	var wg sync.WaitGroup
	overAllocated := atomic.Int64{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job.staticIpMu.Lock()
			defer job.staticIpMu.Unlock()
			if job.lenStaticIps() < 5 {
				ip := fmt.Sprintf("10.0.1.%d", overAllocated.Load()+1)
				job.staticIps.Store(ip, types.StaticIp{})
				overAllocated.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := overAllocated.Load(); got != 1 {
		t.Errorf("expected exactly 1 allocation, got %d", got)
	}
	if got := job.lenStaticIps(); got != 5 {
		t.Errorf("expected 5 static IPs, got %d", got)
	}
}

// updatePeak atomically tracks the peak value seen across goroutines.
func updatePeak(peak *atomic.Int64, cur int64) {
	for {
		old := peak.Load()
		if cur <= old || peak.CompareAndSwap(old, cur) {
			return
		}
	}
}

func TestRunConcurrency_ParallelExecution(t *testing.T) {
	var current atomic.Int64
	var peak atomic.Int64
	var wg sync.WaitGroup

	concurrency := 5
	sem := make(chan struct{}, concurrency)

	start := time.Now()
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cur := current.Add(1)
			updatePeak(&peak, cur)
			time.Sleep(100 * time.Millisecond)
			current.Add(-1)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 300*time.Millisecond {
		t.Errorf("expected parallel execution (<300ms), took %v", elapsed)
	}
	if p := peak.Load(); p < 2 {
		t.Errorf("expected peak concurrency >= 2, got %d", p)
	}
}

func TestRunConcurrencyLimit(t *testing.T) {
	maxConcurrency := 3
	var current atomic.Int64
	var peak atomic.Int64
	var wg sync.WaitGroup

	sem := make(chan struct{}, maxConcurrency)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cur := current.Add(1)
			updatePeak(&peak, cur)
			time.Sleep(50 * time.Millisecond)
			current.Add(-1)
		}()
	}
	wg.Wait()

	if p := peak.Load(); p > int64(maxConcurrency) {
		t.Errorf("peak concurrency %d exceeded limit %d", p, maxConcurrency)
	}
}
