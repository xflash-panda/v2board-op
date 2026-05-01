package change_ip

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/cloudflare/cloudflare-go"
	log "github.com/sirupsen/logrus"
	"github.com/theckman/go-flock"
	"golang.org/x/sync/errgroup"

	"github.com/xflash-panda/v2board-op/internal/pkg/api"
	"github.com/xflash-panda/v2board-op/internal/pkg/service"
	"github.com/xflash-panda/v2board-op/internal/pkg/util"
)

const (
	defaultPingRetries = 3
	defaultSleepTime   = 8 * time.Second
	defaultBatchSize   = 3
)

type Ip *string

type LightsailCFJobConfig struct {
	QueryTags   []string
	Concurrency int
}

type LightsailJobStats struct {
	total   int
	success atomic.Int64
	fail    atomic.Int64
}

func (conf *LightsailCFJobConfig) check() error {
	// TODO Check region enumeration
	if len(conf.QueryTags) == 0 {
		return fmt.Errorf("configuration error: %s", "query tags is empty")
	}

	if conf.Concurrency < 0 {
		return fmt.Errorf("configuration error: %s", "concurrency must be non-negative")
	}
	return nil
}

type LightsailCFJob struct {
	lightsailSrv  *service.LightSailService
	cloudflareSrv *service.CloudflareService
	apiClient     *api.Client
	conf          *LightsailCFJobConfig
	zoneByRoot    sync.Map // rootDomain -> zoneId
	zoneByHost    sync.Map // host -> zoneId
	instances     sync.Map
	staticIps     sync.Map
	staticIpMu    sync.Mutex
	dnsRecords    sync.Map // dnsKey(host, ip) -> cloudflare.DNSRecord
	preparedHosts map[string]struct{}
	lock          *flock.Flock
	stats         *LightsailJobStats
}

func dnsKey(host, ip string) string { return host + "|" + ip }

// uniqueHosts collects the deduplicated set of hosts referenced by the
// banned list.
func uniqueHosts(items []*api.BannedHostInfo) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it.Host == "" {
			continue
		}
		if _, ok := seen[it.Host]; ok {
			continue
		}
		seen[it.Host] = struct{}{}
		out = append(out, it.Host)
	}
	return out
}

func NewLightsailCFJob(conf *LightsailCFJobConfig, apiClient *api.Client, lightsailSrv *service.LightSailService, cloudflareSrv *service.CloudflareService) *LightsailCFJob {
	stats := &LightsailJobStats{}
	lock := flock.New(lockFilePath(conf))
	return &LightsailCFJob{
		conf:          conf,
		apiClient:     apiClient,
		lightsailSrv:  lightsailSrv,
		cloudflareSrv: cloudflareSrv,
		stats:         stats,
		lock:          lock,
		preparedHosts: make(map[string]struct{}),
	}
}

// lockFilePath derives a stable lock path from QueryTags so the job can run
// regardless of how many domains it covers.
func lockFilePath(conf *LightsailCFJobConfig) string {
	tags := append([]string(nil), conf.QueryTags...)
	sort.Strings(tags)
	sum := sha256.Sum256([]byte(strings.Join(tags, ",")))
	return fmt.Sprintf("/tmp/change_ip_%s.lock", hex.EncodeToString(sum[:8]))
}

func (m *LightsailCFJob) Init() error {
	log.Infoln("job is initializing")
	if err := m.conf.check(); err != nil {
		return err
	}
	if err := m.initInstances(); err != nil {
		return err
	}

	if err := m.initStaticIps(); err != nil {
		return err
	}

	// DNS records are loaded lazily per host once the banned list is known,
	// since hosts (and their zones) are now per-record rather than a single
	// job-wide domain.

	log.Infoln("job initialization completed")
	return nil
}

func (m *LightsailCFJob) initInstances() error {
	lightsailClient, err := m.lightsailSrv.GetClient()
	if err != nil {
		return fmt.Errorf("init instances error: %s", err)
	}

	var pageToken *string
	total := 0
	for {
		instancesOutput, err := lightsailClient.GetInstances(context.TODO(), &lightsail.GetInstancesInput{PageToken: pageToken})
		if err != nil {
			return fmt.Errorf("init instances error: %s", err)
		}

		for _, instance := range instancesOutput.Instances {
			if instance.PublicIpAddress != nil {
				m.instances.Store(*instance.PublicIpAddress, instance)
			}
		}
		total += len(instancesOutput.Instances)

		if instancesOutput.NextPageToken == nil {
			break
		}
		pageToken = instancesOutput.NextPageToken
	}
	log.Infof("Get %d VPS instances", total)
	return nil
}

func (m *LightsailCFJob) initStaticIps() error {
	lightsailClient, err := m.lightsailSrv.GetClient()
	if err != nil {
		return fmt.Errorf("init static ips error: %s", err)
	}

	var pageToken *string
	total := 0
	for {
		staticIpsOutput, err := lightsailClient.GetStaticIps(context.TODO(), &lightsail.GetStaticIpsInput{PageToken: pageToken})
		if err != nil {
			return err
		}

		for _, staticIp := range staticIpsOutput.StaticIps {
			if staticIp.IpAddress != nil {
				m.staticIps.Store(*staticIp.IpAddress, staticIp)
			}
		}
		total += len(staticIpsOutput.StaticIps)

		if staticIpsOutput.NextPageToken == nil {
			break
		}
		pageToken = staticIpsOutput.NextPageToken
	}

	log.Infof("Get %d static ips", total)
	return nil
}

// resolveZoneId returns the Cloudflare zone ID that owns `host`, caching
// results both by root domain and by host so each unique zone is only looked
// up once.
func (m *LightsailCFJob) resolveZoneId(host string) (string, error) {
	if v, ok := m.zoneByHost.Load(host); ok {
		return v.(string), nil
	}
	rootDomain, err := util.GetRootDomain(host)
	if err != nil {
		return "", fmt.Errorf("resolve zone for %s: %s", host, err)
	}
	if v, ok := m.zoneByRoot.Load(rootDomain); ok {
		zoneId := v.(string)
		m.zoneByHost.Store(host, zoneId)
		return zoneId, nil
	}
	dnsClient, err := m.cloudflareSrv.GetAPI()
	if err != nil {
		return "", fmt.Errorf("resolve zone for %s: %s", host, err)
	}
	zones, err := dnsClient.ListZones(context.TODO(), rootDomain)
	if err != nil {
		return "", fmt.Errorf("resolve zone for %s: %s", host, err)
	}
	if len(zones) != 1 {
		return "", fmt.Errorf("resolve zone for %s: expected 1 zone for %s, got %d", host, rootDomain, len(zones))
	}
	zoneId := zones[0].ID
	m.zoneByRoot.Store(rootDomain, zoneId)
	m.zoneByHost.Store(host, zoneId)
	return zoneId, nil
}

// prepareDnsForHosts loads existing A records for every unique host in the
// banned list and caches them by (host, ip). Called serially from Run before
// goroutines fan out, so it does not coordinate concurrent callers.
// Idempotent: hosts already prepared are skipped.
func (m *LightsailCFJob) prepareDnsForHosts(hosts []string) error {
	dnsClient, err := m.cloudflareSrv.GetAPI()
	if err != nil {
		return fmt.Errorf("prepare dns records error: %s", err)
	}

	for _, host := range hosts {
		if host == "" {
			continue
		}
		if _, ok := m.preparedHosts[host]; ok {
			continue
		}
		zoneId, err := m.resolveZoneId(host)
		if err != nil {
			return err
		}
		records, _, err := dnsClient.ListDNSRecords(context.TODO(), cloudflare.ZoneIdentifier(zoneId), cloudflare.ListDNSRecordsParams{
			Name: host,
			Type: "A",
		})
		if err != nil {
			return fmt.Errorf("list dns records for %s: %s", host, err)
		}
		for _, record := range records {
			m.dnsRecords.Store(dnsKey(host, record.Content), record)
		}
		m.preparedHosts[host] = struct{}{}
		log.Infof("Loaded %d DNS records for %s", len(records), host)
	}
	return nil
}

func (m *LightsailCFJob) Run() (rerunState bool, err error) {
	locked, _ := m.lock.TryLock()
	if !locked {
		return false, fmt.Errorf("%s(%s)", "the program is running, please release the lock file", m.lock.Path())
	}

	defer func() {
		if err := m.lock.Unlock(); err != nil {
			log.Printf("failed to unlock: %v", err)
		}
	}()
	log.Infoln("Job is running")

	bannedList, err := m.apiClient.QueryBannedList(m.conf.QueryTags)
	if err != nil {
		return false, fmt.Errorf("query banned list error: %s", err)
	}

	bannedListLen := len(bannedList)
	if bannedListLen == 0 {
		log.Infoln("banned list is empty")
		return false, nil
	}

	m.stats.total = bannedListLen
	log.Infof("Found %d walled hosts", bannedListLen)

	if err := m.prepareDnsForHosts(uniqueHosts(bannedList)); err != nil {
		return false, fmt.Errorf("prepare dns records error: %s", err)
	}

	batchSize := m.conf.Concurrency
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	for i := 0; i < bannedListLen; i += batchSize {
		end := i + batchSize
		if end > bannedListLen {
			end = bannedListLen
		}
		batch := bannedList[i:end]
		log.Infof("Processing batch %d-%d / %d", i+1, end, bannedListLen)

		// Pre-check: filter out hosts that are actually reachable (false positive from service restart)
		pingResults := m.batchPing(batch)
		var unreachable []*api.BannedHostInfo
		for _, item := range batch {
			key := fmt.Sprintf("%s:%d", item.IP, item.Port)
			if pingResults[key] {
				log.Infof("Host %s is reachable, skip", key)
			} else {
				unreachable = append(unreachable, item)
			}
		}
		if len(unreachable) == 0 {
			log.Infof("All hosts in batch are reachable, skip")
			continue
		}

		g := new(errgroup.Group)
		for _, bannedItem := range unreachable {
			item := bannedItem
			g.Go(func() error {
				m.processBannedItem(item)
				return nil
			})
		}
		_ = g.Wait()
	}

	if m.stats.fail.Load() > 0 {
		return true, nil
	}

	return false, nil
}

// batchPing calls the batch ping API with retries. Returns a map of "host:port" => reachable.
// On persistent failure, returns nil (caller should treat all as unreachable).
func (m *LightsailCFJob) batchPing(items []*api.BannedHostInfo) map[string]bool {
	hosts := make([]api.PingBatchHost, len(items))
	for i, item := range items {
		hosts[i] = api.PingBatchHost{Host: item.IP, Port: item.Port}
	}

	for attempt := 1; attempt <= defaultPingRetries; attempt++ {
		results, err := m.apiClient.TestPingBatch(hosts)
		if err != nil {
			log.Warnf("Batch ping request failed (%d/%d): %v", attempt, defaultPingRetries, err)
			time.Sleep(defaultSleepTime)
			continue
		}
		return results
	}

	log.Errorf("Batch ping failed after %d retries", defaultPingRetries)
	return nil
}

func (m *LightsailCFJob) processBannedItem(bannedItem *api.BannedHostInfo) {
	log.Infof("current banned host: %s", bannedItem)
	host := bannedItem.Host
	if host == "" {
		// Aborting before any side effect keeps panel + DNS state coherent:
		// rotating the IP and notifying the panel without updating DNS would
		// leave Cloudflare pointing at the stale (banned) IP indefinitely.
		m.stats.fail.Add(1)
		log.Errorf("Banned item %s has no host; aborting to avoid stale DNS", bannedItem)
		return
	}
	instance, ok := m.instances.Load(bannedItem.IP)
	if !ok {
		log.Warnf("No instance found, skip %s", bannedItem)
		if err := m.dropDnsRecord(host, bannedItem.IP); err != nil {
			log.Error(err)
		}
		return
	}
	log.Infof("found instance: %s", bannedItem)

	newIp, err := m.processInstance(instance.(types.Instance))
	if err != nil {
		log.Errorf("Process instance failed: %s", err)
		return
	}

	log.Infof("%s Getting a new ip: %s", bannedItem, *newIp)

	// Verify new IP is reachable
	checkItem := &api.BannedHostInfo{IP: *newIp, Port: bannedItem.Port}
	pingResult := m.batchPing([]*api.BannedHostInfo{checkItem})
	if pingResult[fmt.Sprintf("%s:%d", *newIp, bannedItem.Port)] {
		m.stats.success.Add(1)
		log.Infof("New IP %s is reachable", *newIp)
	} else {
		m.stats.fail.Add(1)
		log.Warnf("New IP %s is NOT reachable", *newIp)
	}

	err = m.changeIp(bannedItem, *newIp)
	if err != nil {
		log.Errorf("Change ip error: %s", err)
	}
	log.Infoln("change ip success")
	if err = m.dropDnsRecord(host, bannedItem.IP); err != nil {
		log.Error(err)
	} else {
		log.Infof("delete dns record {%s: %s}", host, bannedItem.IP)
	}
	err = m.createDnsRecord(host, *newIp)
	if err != nil {
		log.Error(err)
	} else {
		log.Infof("Add dns record %s : %s", host, *newIp)
	}
}

func (m *LightsailCFJob) dropDnsRecord(host, ip string) error {
	dnsClient, _ := m.cloudflareSrv.GetAPI()
	key := dnsKey(host, ip)
	record, ok := m.dnsRecords.Load(key)
	if !ok {
		return fmt.Errorf("not found dns record from cache {%s: %s}", host, ip)
	}
	zoneId, err := m.resolveZoneId(host)
	if err != nil {
		return err
	}
	rec := record.(cloudflare.DNSRecord)
	if err := dnsClient.DeleteDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(zoneId), rec.ID); err != nil {
		return fmt.Errorf("delete dns {%s: %s} record error: %s", rec.Name, rec.Content, err)
	}
	m.dnsRecords.Delete(key)
	return nil
}

func (m *LightsailCFJob) createDnsRecord(host, ip string) error {
	dnsClient, _ := m.cloudflareSrv.GetAPI()
	zoneId, err := m.resolveZoneId(host)
	if err != nil {
		return err
	}
	record, err := dnsClient.CreateDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(zoneId), cloudflare.CreateDNSRecordParams{
		Type:    "A",
		Name:    host,
		Content: ip,
		TTL:     60,
	})
	if err != nil {
		return fmt.Errorf("add dns record %s: %s error: %s", host, ip, err)
	}
	m.dnsRecords.Store(dnsKey(host, ip), record)
	return nil
}

func (m *LightsailCFJob) changeIp(bannedItem *api.BannedHostInfo, newIp string) error {
	return m.apiClient.ChangeIP(bannedItem.Type, bannedItem.ID, bannedItem.IP, newIp)
}

func (m *LightsailCFJob) processInstance(instance types.Instance) (newIP *string, err error) {
	log.Infof("process instance {name: %s, ip: %s}", *instance.Name, *instance.PublicIpAddress)

	if *instance.IsStaticIp {
		log.Infof("{name: %s, ip: %s} must be release static ip", *instance.Name, *instance.PublicIpAddress)
		m.staticIpMu.Lock()
		newIP, err = m.processInstanceReleaseStaticIp(instance)
		m.staticIpMu.Unlock()
		return newIP, err
	}

	m.staticIpMu.Lock()
	if m.lenStaticIps() >= service.LightsailMaxStaticIp {
		m.staticIpMu.Unlock()
		// mustRestart 不涉及静态 IP，无需持锁，可并行执行
		log.Infof("{name: %s, ip: %s} must be restart to get new IP", *instance.Name, *instance.PublicIpAddress)
		return m.processInstanceMustRestart(instance)
	}
	log.Infof("{name: %s, ip: %s} need to allocate a new static IP", *instance.Name, *instance.PublicIpAddress)
	newIP, err = m.processInstanceAllocateStaticIp(instance)
	m.staticIpMu.Unlock()
	return newIP, err
}

func (m *LightsailCFJob) processInstanceReleaseStaticIp(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	staticIp, ok := m.staticIps.Load(*instance.PublicIpAddress)
	if !ok {
		return nil, fmt.Errorf("no static ip found, %s", *instance.PublicIpAddress)
	}
	staticIpName := staticIp.(types.StaticIp).Name
	_, err = lightsailClient.DetachStaticIp(context.TODO(), &lightsail.DetachStaticIpInput{StaticIpName: staticIpName})
	if err != nil {
		return nil, err
	}
	_, err = lightsailClient.ReleaseStaticIp(context.TODO(), &lightsail.ReleaseStaticIpInput{StaticIpName: staticIpName})
	if err != nil {
		return nil, err
	}

	m.staticIps.Delete(*instance.PublicIpAddress)

	instanceOutput, err := lightsailClient.GetInstance(context.TODO(), &lightsail.GetInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}

	newIp = instanceOutput.Instance.PublicIpAddress

	return newIp, nil
}

func (m *LightsailCFJob) processInstanceAllocateStaticIp(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	staticIpName := fmt.Sprintf("%s-static-ip", *instance.Name)
	_, err = lightsailClient.AllocateStaticIp(context.TODO(), &lightsail.AllocateStaticIpInput{StaticIpName: &staticIpName})
	if err != nil {
		return nil, err
	}

	_, err = lightsailClient.AttachStaticIp(context.TODO(), &lightsail.AttachStaticIpInput{InstanceName: instance.Name, StaticIpName: &staticIpName})
	if err != nil {
		return nil, err
	}

	staticIpOutput, err := lightsailClient.GetStaticIp(context.TODO(), &lightsail.GetStaticIpInput{StaticIpName: &staticIpName})
	if err != nil {
		return nil, err
	}

	newIp = staticIpOutput.StaticIp.IpAddress
	if newIp != nil {
		m.staticIps.Store(*newIp, *staticIpOutput.StaticIp)
	}
	return newIp, nil
}

func (m *LightsailCFJob) processInstanceMustRestart(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	_, err = lightsailClient.StopInstance(context.TODO(), &lightsail.StopInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}
	log.Infof("{name: %s} is stoping", *instance.Name)
	stateName := ""
	for stateName != "stopped" {
		stateOutput, err := lightsailClient.GetInstanceState(context.TODO(), &lightsail.GetInstanceStateInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			return nil, err
		}
		stateName = *stateOutput.State.Name
		log.Infof("{name: %s} state is %s", *instance.Name, *stateOutput.State.Name)
		time.Sleep(defaultSleepTime)
	}
	log.Infof("{name: %s} is stopped", *instance.Name)
	_, err = lightsailClient.StartInstance(context.TODO(), &lightsail.StartInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}

	log.Infof("{name: %s} is starting", *instance.Name)
	for stateName != "running" {
		stateOutput, err := lightsailClient.GetInstanceState(context.TODO(), &lightsail.GetInstanceStateInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			return nil, err
		}
		stateName = *stateOutput.State.Name
		log.Infof("{name: %s, ip: %s} state is %s", *instance.Name, *instance.PublicIpAddress, *stateOutput.State.Name)
		time.Sleep(defaultSleepTime)
	}
	log.Infof("{name: %s} is running", *instance.Name)

	instanceOutput, err := lightsailClient.GetInstance(context.TODO(), &lightsail.GetInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}
	newIp = instanceOutput.Instance.PublicIpAddress
	return newIp, nil
}

func (m *LightsailCFJob) lenStaticIps() int {
	i := 0
	m.staticIps.Range(func(k, v any) bool {
		i++
		return true
	})
	return i
}
