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

func (c *LightsailSrvConfig) Check() error {
	if len(c.Region) == 0 {
		return fmt.Errorf("configuration error: %s", "lightsail region  is empty")
	}
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
}

func NewLightSailService(conf *LightsailSrvConfig) (*LightSailService, error) {
	if err := conf.Check(); err != nil {
		return nil, err
	}
	return &LightSailService{conf: conf}, nil
}

func (s *LightSailService) GetClient() (*lightsail.Client, error) {
	s.clientOnce.Do(func() {
		var cfg aws.Config
		if s.conf.isStaticKeySecret() {
			cfg, s.clientErr = config.LoadDefaultConfig(context.TODO(), config.WithRegion(s.conf.Region), config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(s.conf.Key, s.conf.Secret, "")))
		} else {
			cfg, s.clientErr = config.LoadDefaultConfig(context.TODO(), config.WithRegion(s.conf.Region))
		}
		if s.clientErr != nil {
			return
		}
		s.client = lightsail.NewFromConfig(cfg)
	})
	return s.client, s.clientErr
}
