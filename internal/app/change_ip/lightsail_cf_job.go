package change_ip

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/cloudflare/cloudflare-go"
	log "github.com/sirupsen/logrus"
	"github.com/theckman/go-flock"
	"github.com/xflash-panda/v2board-op/internal/pkg/api"
	"github.com/xflash-panda/v2board-op/internal/pkg/service"
	"github.com/xflash-panda/v2board-op/internal/pkg/util"
)

const (
	defaultPingTryNum = 5
	defaultSleepTime  = 8 * time.Second
)

type Ip *string

type LightsailCFJobConfig struct {
	QueryTags   []string
	Domain      string
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

	if len(conf.Domain) == 0 {
		return fmt.Errorf("configuration error: %s", "domain is empty")
	}

	if !util.IsValidDomain(conf.Domain) {
		return fmt.Errorf("configuration error: %s", "invalid domain name")
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
	dnsZoneId     string
	instances     sync.Map
	staticIps     sync.Map
	staticIpMu    sync.Mutex
	dnsRecords    sync.Map
	lock          *flock.Flock
	stats         *LightsailJobStats
}

func NewLightsailCFJob(conf *LightsailCFJobConfig, apiClient *api.Client, lightsailSrv *service.LightSailService, cloudflareSrv *service.CloudflareService) *LightsailCFJob {
	stats := &LightsailJobStats{}
	lockPath := fmt.Sprintf("/tmp/change_ip_%s.lock", strings.ReplaceAll(conf.Domain, ".", "_"))
	lock := flock.New(lockPath)
	return &LightsailCFJob{conf: conf, apiClient: apiClient, lightsailSrv: lightsailSrv, cloudflareSrv: cloudflareSrv, stats: stats, lock: lock}
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

	if err := m.initDnsRecords(); err != nil {
		return err
	}

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

func (m *LightsailCFJob) initDnsRecords() error {
	rootDomain, err := util.GetRootDomain(m.conf.Domain)
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	dnsClient, err := m.cloudflareSrv.GetAPI()
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	zones, err := dnsClient.ListZones(context.TODO(), rootDomain)
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	if len(zones) != 1 {
		return fmt.Errorf("init dns records error: %s", "dns zones return value error")
	}

	zoneId := zones[0].ID
	m.dnsZoneId = zoneId
	records, _, err := dnsClient.ListDNSRecords(context.TODO(), cloudflare.ZoneIdentifier(zoneId), cloudflare.ListDNSRecordsParams{
		Name: m.conf.Domain,
		Type: "A",
	})

	log.Infof("Get %d DNS records", len(records))

	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	for _, record := range records {
		m.dnsRecords.Store(record.Content, record)
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
		return false, fmt.Errorf("Query banned List error:  %s", err)
	}

	bannedListLen := len(bannedList)
	if bannedListLen == 0 {
		log.Infoln("banned list is empty")
		return false, nil
	}

	m.stats.total = bannedListLen
	log.Infof("Found %d walled hosts", bannedListLen)
	for _, bannedItem := range bannedList {
		log.Infof("current banned host: %s", bannedItem)
		instance, ok := m.instances.Load(bannedItem.IP)
		if !ok {
			log.Errorf("No instance found, %s ", bannedItem)
			err = m.dropDnsRecord(bannedItem.IP)
			if err != nil {
				log.Error(err)
			}
			continue
		}
		log.Infof("found instance: %s", bannedItem)

		checkPingResult, err := m.testPing(bannedItem.IP, bannedItem.Port, defaultPingTryNum)
		if err != nil {
			log.Errorf("check ping error, %s", err)
			continue
		}
		log.Infof("Check ping result: %v", checkPingResult)
		if checkPingResult {
			if err = m.changeIp(bannedItem, bannedItem.IP, checkPingResult); err != nil {
				log.Errorf("Change ip error: %s", err)
			}
			continue
		}

		newIp, err := m.processInstance(instance.(types.Instance))
		if err != nil {
			log.Errorf("Process instance failed: %s", err)
			continue
		}

		log.Infof("%s Getting a new ip: %s", bannedItem, *newIp)
		pingResult, err := m.testPing(*newIp, bannedItem.Port, defaultPingTryNum)
		if err != nil {
			log.Errorf("Test ping failed: %s", err)
		}
		log.Infof("Ip %s ping result is %v ", *newIp, pingResult)

		if pingResult {
			m.stats.success.Add(1)
		} else {
			m.stats.fail.Add(1)
		}

		err = m.changeIp(bannedItem, *newIp, pingResult)
		if err != nil {
			log.Errorf("Change ip error: %s", err)
		}
		log.Infoln("change ip success")
		if ok {
			if err = m.dropDnsRecord(bannedItem.IP); err != nil {
				log.Error(err)
			} else {
				log.Infof("delete dns record {%s: %s}", m.conf.Domain, bannedItem.IP)
			}
		}
		err = m.createDnsRecord(*newIp)
		if err != nil {
			log.Error(err)
		} else {
			log.Infof("Add dns record %s : %s", m.conf.Domain, *newIp)
		}

	}

	if m.stats.fail.Load() > 0 {
		return true, nil
	}

	return false, nil
}

func (m *LightsailCFJob) dropDnsRecord(ip string) error {
	dnsClient, _ := m.cloudflareSrv.GetAPI()
	record, ok := m.dnsRecords.Load(ip)
	if !ok {
		return fmt.Errorf("not found %s dns record from cache", ip)
	}
	err := dnsClient.DeleteDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(m.dnsZoneId), record.(cloudflare.DNSRecord).ID)
	if err != nil {
		return fmt.Errorf("delete dns {%s - %s: %s} record error: %s", m.dnsZoneId, record.(cloudflare.DNSRecord).Name, record.(cloudflare.DNSRecord).Content, err)
	}
	return nil
}

func (m *LightsailCFJob) createDnsRecord(ip string) error {
	dnsClient, _ := m.cloudflareSrv.GetAPI()
	if len(m.dnsZoneId) == 0 {
		return fmt.Errorf("uninitialized region ID")
	}
	_, err := dnsClient.CreateDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(m.dnsZoneId), cloudflare.CreateDNSRecordParams{
		Type:    "A",
		Name:    m.conf.Domain,
		Content: ip,
		TTL:     60,
	})

	if err != nil {
		return fmt.Errorf("Add dns record  %s : %s error: %s", m.conf.Domain, ip, err)
	}
	return nil
}

func (m *LightsailCFJob) testPing(host string, port int, tryNum int) (bool, error) {
	i := 0
	var pingResult api.PingResult
	var err error
	for i < tryNum {
		pingResult, err = m.apiClient.TestPing(host, port)
		i++
		log.Infof("Host %s:%d  ping result is %v  %d times", host, port, pingResult, i)
		if err != nil {
			return false, err
		}
		if pingResult {
			break
		}
		time.Sleep(defaultSleepTime)
	}
	return bool(pingResult), nil
}

func (m *LightsailCFJob) changeIp(bannedItem *api.BannedHostInfo, newIp string, pingResult bool) error {
	return m.apiClient.ChangeIP(bannedItem.Type, bannedItem.ID, bannedItem.IP, newIp, !pingResult)
}

func (m *LightsailCFJob) processInstance(instance types.Instance) (newIP *string, err error) {
	m.staticIpMu.Lock()
	defer m.staticIpMu.Unlock()
	log.Infof("process instance {name: %s, ip: %s}", *instance.Name, *instance.PublicIpAddress)
	if *instance.IsStaticIp {
		log.Infof("{name: %s, ip: %s} must be release static ip", *instance.Name, *instance.PublicIpAddress)
		return m.processInstanceReleaseStaticIp(instance)
	}
	//Allocate Static Ip
	if m.lenStaticIps() >= service.LightsailMaxStaticIp {
		log.Infof("{name: %s, ip: %s} must be restart to get new IP", *instance.Name, *instance.PublicIpAddress)
		return m.processInstanceMustRestart(instance)
	}
	log.Infof("{name: %s, ip: %s} need to allocate a new static IP", *instance.Name, *instance.PublicIpAddress)
	return m.processInstanceAllocateStaticIp(instance)
}

func (m *LightsailCFJob) processInstanceReleaseStaticIp(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	staticIp, ok := m.staticIps.Load(*instance.PublicIpAddress)
	if !ok {
		return nil, fmt.Errorf("No static ip found, %s ", *instance.PublicIpAddress)
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
