package service

import (
	"context"
	"testing"
)

func TestLightsailSrvConfig_Check_EmptyRegionAllowed(t *testing.T) {
	conf := &LightsailSrvConfig{}
	if err := conf.Check(); err != nil {
		t.Fatalf("empty Region should be allowed, got: %v", err)
	}
}

func TestGetClientForRegion_SameRegionCached(t *testing.T) {
	svc, err := NewLightSailService(&LightsailSrvConfig{})
	if err != nil {
		t.Fatalf("NewLightSailService: %v", err)
	}
	c1, err := svc.GetClientForRegion(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	c2, err := svc.GetClientForRegion(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if c1 != c2 {
		t.Errorf("expected cached client to be identical pointer")
	}
}

func TestGetClientForRegion_DifferentRegionsDifferentClients(t *testing.T) {
	svc, err := NewLightSailService(&LightsailSrvConfig{})
	if err != nil {
		t.Fatalf("NewLightSailService: %v", err)
	}
	c1, err := svc.GetClientForRegion(context.Background(), "us-east-1")
	if err != nil {
		t.Fatalf("us-east-1: %v", err)
	}
	c2, err := svc.GetClientForRegion(context.Background(), "ap-northeast-1")
	if err != nil {
		t.Fatalf("ap-northeast-1: %v", err)
	}
	if c1 == c2 {
		t.Errorf("expected distinct clients for different regions")
	}
}
