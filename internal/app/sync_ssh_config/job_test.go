package sync_ssh_config

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
)

// fakeAPI implements LightsailAPI for tests.
type fakeAPI struct {
	regionsOut    *lightsail.GetRegionsOutput
	regionsErr    error
	instancesOut  map[string]*lightsail.GetInstancesOutput // optFns "Region" key -> output
	instancesErr  map[string]error
	instancesCall map[string]int
}

func (f *fakeAPI) GetRegions(ctx context.Context, params *lightsail.GetRegionsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetRegionsOutput, error) {
	return f.regionsOut, f.regionsErr
}

func (f *fakeAPI) GetInstances(ctx context.Context, params *lightsail.GetInstancesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
	opts := &lightsail.Options{}
	for _, fn := range optFns {
		fn(opts)
	}
	region := opts.Region
	f.instancesCall[region]++
	if err, ok := f.instancesErr[region]; ok {
		return nil, err
	}
	return f.instancesOut[region], nil
}

// fakeFactory returns the same fakeAPI for every region but tags the call
// with the requested region so the fakeAPI knows which output to return.
func fakeFactory(api *fakeAPI) ClientFactory {
	return func(ctx context.Context, region string) (LightsailAPI, error) {
		// Wrap so optFns inside FetchInstances see the right region.
		return &regionTaggedAPI{inner: api, region: region}, nil
	}
}

type regionTaggedAPI struct {
	inner  *fakeAPI
	region string
}

func (r *regionTaggedAPI) GetRegions(ctx context.Context, params *lightsail.GetRegionsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetRegionsOutput, error) {
	return r.inner.GetRegions(ctx, params, optFns...)
}
func (r *regionTaggedAPI) GetInstances(ctx context.Context, params *lightsail.GetInstancesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
	withRegion := append([]func(*lightsail.Options){
		func(o *lightsail.Options) { o.Region = r.region },
	}, optFns...)
	return r.inner.GetInstances(ctx, params, withRegion...)
}

func TestFetchInstances_MultipleRegionsMerged(t *testing.T) {
	api := &fakeAPI{
		regionsOut: &lightsail.GetRegionsOutput{
			Regions: []types.Region{
				{Name: types.RegionNameUsEast1},
				{Name: types.RegionNameApNortheast1},
			},
		},
		instancesOut: map[string]*lightsail.GetInstancesOutput{
			"us-east-1": {
				Instances: []types.Instance{{Name: aws.String("alpha"), PublicIpAddress: aws.String("1.1.1.1")}},
			},
			"ap-northeast-1": {
				Instances: []types.Instance{{Name: aws.String("beta"), PublicIpAddress: aws.String("2.2.2.2")}},
			},
		},
		instancesErr:  map[string]error{},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	got, err := job.FetchInstances(context.Background())
	if err != nil {
		t.Fatalf("FetchInstances: %v", err)
	}
	if got["alpha"] != "1.1.1.1" || got["beta"] != "2.2.2.2" {
		t.Errorf("got %v", got)
	}
}

func TestFetchInstances_CrossRegionDuplicateDropped(t *testing.T) {
	api := &fakeAPI{
		regionsOut: &lightsail.GetRegionsOutput{
			Regions: []types.Region{
				{Name: types.RegionNameUsEast1},
				{Name: types.RegionNameApNortheast1},
			},
		},
		instancesOut: map[string]*lightsail.GetInstancesOutput{
			"us-east-1":      {Instances: []types.Instance{{Name: aws.String("dup"), PublicIpAddress: aws.String("1.1.1.1")}}},
			"ap-northeast-1": {Instances: []types.Instance{{Name: aws.String("dup"), PublicIpAddress: aws.String("2.2.2.2")}}},
		},
		instancesErr:  map[string]error{},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	got, _ := job.FetchInstances(context.Background())
	if _, present := got["dup"]; present {
		t.Errorf("duplicate-named instance should be dropped, got %v", got)
	}
}

func TestFetchInstances_SingleRegionFailureContinues(t *testing.T) {
	api := &fakeAPI{
		regionsOut: &lightsail.GetRegionsOutput{
			Regions: []types.Region{
				{Name: types.RegionNameUsEast1},
				{Name: types.RegionNameApNortheast1},
			},
		},
		instancesOut: map[string]*lightsail.GetInstancesOutput{
			"ap-northeast-1": {Instances: []types.Instance{{Name: aws.String("alpha"), PublicIpAddress: aws.String("1.1.1.1")}}},
		},
		instancesErr: map[string]error{
			"us-east-1": errors.New("boom"),
		},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	got, err := job.FetchInstances(context.Background())
	if err != nil {
		t.Fatalf("partial failure should not error, got %v", err)
	}
	if got["alpha"] != "1.1.1.1" {
		t.Errorf("got %v want alpha=1.1.1.1", got)
	}
}

func TestFetchInstances_AllRegionsFailErrors(t *testing.T) {
	api := &fakeAPI{
		regionsOut: &lightsail.GetRegionsOutput{
			Regions: []types.Region{
				{Name: types.RegionNameUsEast1},
				{Name: types.RegionNameApNortheast1},
			},
		},
		instancesOut: map[string]*lightsail.GetInstancesOutput{},
		instancesErr: map[string]error{
			"us-east-1":      errors.New("boom1"),
			"ap-northeast-1": errors.New("boom2"),
		},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	_, err := job.FetchInstances(context.Background())
	if err == nil {
		t.Fatal("expected error when all regions fail")
	}
	if !strings.Contains(err.Error(), "all") {
		t.Errorf("error should mention 'all', got %v", err)
	}
}

func TestFetchInstances_GetRegionsFailErrors(t *testing.T) {
	api := &fakeAPI{
		regionsErr:    errors.New("network down"),
		instancesOut:  map[string]*lightsail.GetInstancesOutput{},
		instancesErr:  map[string]error{},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	_, err := job.FetchInstances(context.Background())
	if err == nil {
		t.Fatal("expected error when GetRegions fails")
	}
}

type pagingAPI struct {
	regions []types.Region
	pages   map[string][]*lightsail.GetInstancesOutput // region -> ordered pages
	calls   map[string]int
	mu      sync.Mutex
}

func (p *pagingAPI) GetRegions(ctx context.Context, params *lightsail.GetRegionsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetRegionsOutput, error) {
	return &lightsail.GetRegionsOutput{Regions: p.regions}, nil
}

func (p *pagingAPI) GetInstances(ctx context.Context, params *lightsail.GetInstancesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
	opts := &lightsail.Options{}
	for _, fn := range optFns {
		fn(opts)
	}
	region := opts.Region
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.calls[region]
	p.calls[region]++
	pages := p.pages[region]
	if idx >= len(pages) {
		return &lightsail.GetInstancesOutput{}, nil
	}
	return pages[idx], nil
}

func TestFetchInstances_PaginationAcrossPages(t *testing.T) {
	pageToken := "next"
	p := &pagingAPI{
		regions: []types.Region{{Name: types.RegionNameUsEast1}},
		pages: map[string][]*lightsail.GetInstancesOutput{
			"us-east-1": {
				{
					Instances:     []types.Instance{{Name: aws.String("alpha"), PublicIpAddress: aws.String("1.1.1.1")}},
					NextPageToken: &pageToken,
				},
				{
					Instances: []types.Instance{{Name: aws.String("beta"), PublicIpAddress: aws.String("2.2.2.2")}},
				},
			},
		},
		calls: map[string]int{},
	}
	factory := func(ctx context.Context, region string) (LightsailAPI, error) {
		return &pagingTaggedAPI{inner: p, region: region}, nil
	}
	job := NewSyncJob(factory, 4)
	got, err := job.FetchInstances(context.Background())
	if err != nil {
		t.Fatalf("FetchInstances: %v", err)
	}
	if got["alpha"] != "1.1.1.1" || got["beta"] != "2.2.2.2" {
		t.Errorf("pagination not handled: %v", got)
	}
	if p.calls["us-east-1"] != 2 {
		t.Errorf("expected 2 calls for us-east-1, got %d", p.calls["us-east-1"])
	}
}

type pagingTaggedAPI struct {
	inner  *pagingAPI
	region string
}

func (r *pagingTaggedAPI) GetRegions(ctx context.Context, params *lightsail.GetRegionsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetRegionsOutput, error) {
	return r.inner.GetRegions(ctx, params, optFns...)
}
func (r *pagingTaggedAPI) GetInstances(ctx context.Context, params *lightsail.GetInstancesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
	withRegion := append([]func(*lightsail.Options){
		func(o *lightsail.Options) { o.Region = r.region },
	}, optFns...)
	return r.inner.GetInstances(ctx, params, withRegion...)
}

func TestFetchInstances_SingleRegion(t *testing.T) {
	api := &fakeAPI{
		regionsOut: &lightsail.GetRegionsOutput{
			Regions: []types.Region{{Name: types.RegionNameUsEast1}},
		},
		instancesOut: map[string]*lightsail.GetInstancesOutput{
			"us-east-1": {
				Instances: []types.Instance{
					{Name: aws.String("alpha"), PublicIpAddress: aws.String("1.1.1.1")},
					{Name: aws.String("beta"), PublicIpAddress: aws.String("2.2.2.2")},
				},
			},
		},
		instancesErr:  map[string]error{},
		instancesCall: map[string]int{},
	}
	job := NewSyncJob(fakeFactory(api), 4)
	got, err := job.FetchInstances(context.Background())
	if err != nil {
		t.Fatalf("FetchInstances: %v", err)
	}
	if len(got) != 2 || got["alpha"] != "1.1.1.1" || got["beta"] != "2.2.2.2" {
		t.Errorf("got %v want {alpha:1.1.1.1 beta:2.2.2.2}", got)
	}
}
