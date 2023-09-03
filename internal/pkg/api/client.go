package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
	"net/url"
	"strconv"
	"time"
)

// Config  api config
type Config struct {
	APIHost string
	Token   string
	Timeout int
}

// Client APIClient create a api client to the panel.
type Client struct {
	client *resty.Client
	config *Config
}

// New creat a api instance
func New(apiConfig *Config) *Client {
	client := resty.New()
	client.SetRetryCount(3)
	if apiConfig.Timeout > 0 {
		client.SetTimeout(time.Duration(apiConfig.Timeout) * time.Second)
	} else {
		client.SetTimeout(5 * time.Second)
	}
	client.OnError(func(req *resty.Request, err error) {
		var v *resty.ResponseError
		if errors.As(err, &v) {
			// v.Response contains the last response from the server
			// v.Err contains the original error
			log.Errorln(v.Err)
		}
	})
	client.SetBaseURL(apiConfig.APIHost)
	// Create Key for each requests
	client.SetQueryParams(map[string]string{
		"token": apiConfig.Token,
	})

	apiClient := &Client{
		client: client,
		config: apiConfig,
	}
	return apiClient
}

// QueryBannedList Query the list of banned IP
func (c *Client) QueryBannedList(tags []string) (bannedList []*BannedHostInfo, err error) {
	var path = "/api/v1/server/internal/QueryBannedList"
	res, err := c.client.R().
		ForceContentType("application/json").
		SetQueryParamsFromValues(url.Values{
			"tags[]": tags,
		}).
		SetQueryParams(map[string]string{
			"random": strconv.FormatInt(time.Now().Unix(), 10),
		}).
		Get(path)

	if err != nil {
		return nil, fmt.Errorf("request %s failed: %s", c.assembleURL(path), err)
	}

	if res.StatusCode() > 400 {
		body := res.Body()
		return nil, fmt.Errorf("request %s failed: %s, %s", c.assembleURL(path), string(body), err)
	}

	var repBannedList RepQueryBannedList
	if err := json.Unmarshal(res.Body(), &repBannedList); err != nil {
		return nil, fmt.Errorf("parse response failed: %s", err)
	}

	return repBannedList.Data, nil
}

// Debug set the client debug for client
func (c *Client) Debug() {
	c.client.SetDebug(true)
}

func (c *Client) assembleURL(path string) string {
	return c.config.APIHost + path
}

// TestPing  Test if the host port is blocked by a wall
func (c *Client) TestPing(host string, port int) (result PingResult, err error) {
	var path = "/api/v1/server/internal/testPing"
	res, err := c.client.R().
		SetBody(map[string]interface{}{
			"host": host,
			"port": port,
		}).
		ForceContentType("application/json").
		Post(path)

	if res.StatusCode() > 400 {
		body := res.Body()
		return false, fmt.Errorf("request %s failed: %s, %s", c.assembleURL(path), string(body), err)
	}
	var repTestPing RepTestPing
	if err := json.Unmarshal(res.Body(), &repTestPing); err != nil {
		return false, fmt.Errorf("parse response failed: %s", err)
	}

	return repTestPing.Data, nil
}

func (c *Client) ChangeIP(nodeType string, id int, sourceIp string, targetIp string, walledStatus bool) (err error) {
	var path = "/api/v1/server/internal/changeIp"
	res, err := c.client.R().
		SetBody(map[string]interface{}{
			"type":          nodeType,
			"id":            id,
			"source_ip":     sourceIp,
			"target_ip":     targetIp,
			"walled_status": walledStatus,
		}).
		ForceContentType("application/json").
		Post(path)

	if res.StatusCode() > 400 {
		body := res.Body()
		return fmt.Errorf("request %s failed: %s, %s", c.assembleURL(path), string(body), err)
	}

	var repChangeIp RepChangIpResult
	if err := json.Unmarshal(res.Body(), &repChangeIp); err != nil {
		return fmt.Errorf("parse response failed: %s", err)
	}
	return nil
}
