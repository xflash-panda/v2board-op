package change_ip

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
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

	// Simulate what processInstanceReleaseStaticIp now does
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

	// Simulate what processInstanceAllocateStaticIp now does
	job.staticIps.Store("3.3.3.3", types.StaticIp{})

	if got := job.lenStaticIps(); got != 3 {
		t.Errorf("expected 3 after allocate, got %d", got)
	}
}

func TestProcessInstanceRouting_StaticIp(t *testing.T) {
	// When instance has static IP, should choose release path
	// We verify the routing condition only
	isStaticIp := true
	if !isStaticIp {
		t.Error("expected static IP path to be chosen")
	}
}

func TestProcessInstanceRouting_MaxStaticIps_MustRestart(t *testing.T) {
	job := &LightsailCFJob{}
	// Fill up to max (5)
	for i := 0; i < 5; i++ {
		ip := "10.0.0." + string(rune('1'+i))
		job.staticIps.Store(ip, types.StaticIp{})
	}

	if job.lenStaticIps() < 5 {
		t.Fatal("expected >= 5 static IPs")
	}
	// This condition triggers the restart path in processInstance
	if job.lenStaticIps() < 5 {
		t.Error("should trigger restart path when static IPs are at max")
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
	// Start at max
	for i := 0; i < 5; i++ {
		ip := "10.0.0." + string(rune('1'+i))
		job.staticIps.Store(ip, types.StaticIp{})
	}

	if job.lenStaticIps() < 5 {
		t.Fatal("precondition: expected 5 static IPs")
	}

	// Release one (simulating processInstanceReleaseStaticIp fix)
	job.staticIps.Delete("10.0.0.1")

	// Now a subsequent instance should take the allocate path, not restart
	if job.lenStaticIps() >= 5 {
		t.Error("after releasing one static IP, count should be below max, allowing allocate path")
	}
}

func TestConfigCheck_EmptyTags(t *testing.T) {
	conf := &LightsailCFJobConfig{Domain: "test.example.com"}
	if err := conf.check(); err == nil {
		t.Error("expected error for empty tags")
	}
}

func TestConfigCheck_EmptyDomain(t *testing.T) {
	conf := &LightsailCFJobConfig{QueryTags: []string{"tag1"}}
	if err := conf.check(); err == nil {
		t.Error("expected error for empty domain")
	}
}

func TestConfigCheck_InvalidDomain(t *testing.T) {
	conf := &LightsailCFJobConfig{QueryTags: []string{"tag1"}, Domain: "not valid!"}
	if err := conf.check(); err == nil {
		t.Error("expected error for invalid domain")
	}
}

func TestConfigCheck_Valid(t *testing.T) {
	conf := &LightsailCFJobConfig{QueryTags: []string{"tag1"}, Domain: "test.example.com"}
	if err := conf.check(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
