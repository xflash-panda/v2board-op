package service

import (
	"fmt"
	"github.com/cloudflare/cloudflare-go"
)

type CloudflareSrvConfig struct {
	Email string
	Key   string
}

func (c *CloudflareSrvConfig) Check() error {
	if len(c.Email) == 0 {
		return fmt.Errorf("configuration error: %s", "cloudflare email is empty")
	}

	if len(c.Key) == 0 {
		return fmt.Errorf("configuration error: %s", "cloudflare api key is empty")
	}
	return nil
}

type CloudflareService struct {
	conf *CloudflareSrvConfig
	api  *cloudflare.API
}

func NewCloudflareService(conf *CloudflareSrvConfig) (*CloudflareService, error) {
	if err := conf.Check(); err != nil {
		return nil, err
	}
	return &CloudflareService{conf: conf}, nil
}

func (s *CloudflareService) GetAPI() (*cloudflare.API, error) {
	if s.api != nil {
		return s.api, nil
	}
	api, err := cloudflare.New(s.conf.Key, s.conf.Email)
	if err != nil {
		return nil, err
	}
	s.api = api
	return api, nil
}
