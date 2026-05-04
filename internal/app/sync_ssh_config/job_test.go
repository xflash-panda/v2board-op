package sync_ssh_config

import (
	"context"
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
