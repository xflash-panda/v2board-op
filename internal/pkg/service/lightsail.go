package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
)

const (
	LightsailMaxStaticIp int = 5
)

type LightsailSrvConfig struct {
	Key    string
	Secret string
	Region string
}

// Check validates the configuration. Region is intentionally optional here:
// callers using GetClient (single-region) must set Region before the first
// call; callers using GetClientForRegion (multi-region) supply the region
// per call.
func (c *LightsailSrvConfig) Check() error {
	return nil
}

func (c *LightsailSrvConfig) isStaticKeySecret() bool {
	return len(c.Key) > 0 && len(c.Secret) > 0
}

type LightSailService struct {
	conf       *LightsailSrvConfig
	client     *lightsail.Client
	clientOnce sync.Once
	clientErr  error

	regionClients sync.Map // region(string) -> *lightsail.Client
}

func NewLightSailService(conf *LightsailSrvConfig) (*LightSailService, error) {
	if err := conf.Check(); err != nil {
		return nil, err
	}
	return &LightSailService{conf: conf}, nil
}

func (s *LightSailService) loadAWSConfig(ctx context.Context, region string) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{config.WithRegion(region)}
	if s.conf.isStaticKeySecret() {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(s.conf.Key, s.conf.Secret, ""),
		))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

// GetClient returns a memoized Lightsail client using the Region set in
// LightsailSrvConfig. Region must be non-empty before the first call;
// the result of the first call (success or error) is cached permanently.
// For multi-region usage, use GetClientForRegion instead.
func (s *LightSailService) GetClient() (*lightsail.Client, error) {
	s.clientOnce.Do(func() {
		if len(s.conf.Region) == 0 {
			s.clientErr = fmt.Errorf("GetClient requires LightsailSrvConfig.Region to be set; use GetClientForRegion for multi-region usage")
			return
		}
		cfg, err := s.loadAWSConfig(context.TODO(), s.conf.Region)
		if err != nil {
			s.clientErr = err
			return
		}
		s.client = lightsail.NewFromConfig(cfg)
	})
	return s.client, s.clientErr
}

func (s *LightSailService) GetClientForRegion(ctx context.Context, region string) (*lightsail.Client, error) {
	if v, ok := s.regionClients.Load(region); ok {
		return v.(*lightsail.Client), nil
	}
	cfg, err := s.loadAWSConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	client := lightsail.NewFromConfig(cfg)
	actual, _ := s.regionClients.LoadOrStore(region, client)
	return actual.(*lightsail.Client), nil
}
