package sync_ssh_config

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// LightsailAPI is the subset of *lightsail.Client we depend on.
// Defining it here lets tests inject a fake without touching the SDK.
type LightsailAPI interface {
	GetRegions(ctx context.Context, params *lightsail.GetRegionsInput, optFns ...func(*lightsail.Options)) (*lightsail.GetRegionsOutput, error)
	GetInstances(ctx context.Context, params *lightsail.GetInstancesInput, optFns ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error)
}

// ClientFactory builds a LightsailAPI for the given region.
type ClientFactory func(ctx context.Context, region string) (LightsailAPI, error)

// seedRegion is used to call GetRegions; any region works since the API is
// global. We use us-east-1 because it has the highest service availability.
const seedRegion = "us-east-1"

// SyncJob orchestrates concurrent fetching of Lightsail instances across
// every region returned by GetRegions. The result is a single name->IP map
// suitable for diffing against an SSH config.
type SyncJob struct {
	factory     ClientFactory
	concurrency int
}

// NewSyncJob returns a SyncJob configured to query at most `concurrency`
// regions in parallel. A non-positive concurrency defaults to 8.
func NewSyncJob(factory ClientFactory, concurrency int) *SyncJob {
	if concurrency <= 0 {
		concurrency = 8
	}
	return &SyncJob{factory: factory, concurrency: concurrency}
}

// FetchInstances calls GetRegions once, then concurrently calls GetInstances
// in every region, paginating as needed. The returned map is name->PublicIpAddress.
//
// Behavior:
//   - Cross-region duplicate names are dropped (with a warning) to avoid
//     ambiguous updates.
//   - Per-region failures are logged as warnings; FetchInstances returns an
//     error only if every region failed or GetRegions itself failed.
func (j *SyncJob) FetchInstances(ctx context.Context) (map[string]string, error) {
	seedClient, err := j.factory(ctx, seedRegion)
	if err != nil {
		return nil, fmt.Errorf("create seed client: %w", err)
	}
	regionsOut, err := seedClient.GetRegions(ctx, &lightsail.GetRegionsInput{})
	if err != nil {
		return nil, fmt.Errorf("GetRegions: %w", err)
	}

	var (
		mu         sync.Mutex
		instances  = map[string]string{}   // name -> ip
		duplicates = map[string]struct{}{} // names seen in 2+ regions
		successCnt int
		failureCnt int
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(j.concurrency)

	for _, r := range regionsOut.Regions {
		region := string(r.Name)
		g.Go(func() error {
			results, err := j.fetchRegion(gctx, region)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Warnf("region %s GetInstances failed: %v", region, err)
				failureCnt++
				return nil // never fail the group; we summarize after
			}
			successCnt++
			for name, ip := range results {
				if _, dup := duplicates[name]; dup {
					continue
				}
				if existing, seen := instances[name]; seen && existing != ip {
					log.Warnf("instance name %q exists in multiple regions with different IPs (%s, %s); skipping",
						name, existing, ip)
					delete(instances, name)
					duplicates[name] = struct{}{}
					continue
				}
				instances[name] = ip
			}
			return nil
		})
	}
	// Goroutines never return errors (per-region failures are tracked in
	// failureCnt and logged), so g.Wait() always returns nil.
	_ = g.Wait()

	if successCnt == 0 && failureCnt > 0 {
		return nil, fmt.Errorf("all %d regions failed", failureCnt)
	}
	return instances, nil
}

func (j *SyncJob) fetchRegion(ctx context.Context, region string) (map[string]string, error) {
	client, err := j.factory(ctx, region)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	var pageToken *string
	for {
		out, err := client.GetInstances(ctx, &lightsail.GetInstancesInput{PageToken: pageToken})
		if err != nil {
			return nil, err
		}
		for _, inst := range out.Instances {
			if inst.Name == nil || inst.PublicIpAddress == nil {
				continue
			}
			result[*inst.Name] = *inst.PublicIpAddress
		}
		if out.NextPageToken == nil {
			return result, nil
		}
		pageToken = out.NextPageToken
	}
}
