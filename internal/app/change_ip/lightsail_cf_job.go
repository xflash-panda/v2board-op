package change_ip

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/cloudflare/cloudflare-go"
	log "github.com/sirupsen/logrus"
	"github.com/xflash-panda/v2board-op/internal/pkg/api"
	"github.com/xflash-panda/v2board-op/internal/pkg/service"
	"github.com/xflash-panda/v2board-op/internal/pkg/util"
	"sync"
	"time"
)

const (
	defaultPingTryNum = 5
	defaultSleepTime  = 5 * time.Second
)

type Ip *string

type LightsailCFJobConfig struct {
	QueryTags []string
	Domain    string
}

type LightsailJobStats struct {
	total   int
	success int
	fail    int
	skip    int
}

func (conf *LightsailCFJobConfig) check() error {
	// TODO Check region enumeration
	if len(conf.QueryTags) == 0 {
		return fmt.Errorf("configuration error: %s", "query tags is empty")
	}

	if len(conf.Domain) == 0 {
		return fmt.Errorf("configuration error: %s", "domain is empty")
	}

	if util.IsValidDomain(conf.Domain) == false {
		return fmt.Errorf("configuration error: %s", "invalid domain name")
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
	dnsRecords    sync.Map
	access        sync.Mutex
	stats         *LightsailJobStats
}

func NewLightsailCFJob(conf *LightsailCFJobConfig, apiClient *api.Client, lightsailSrv *service.LightSailService, cloudflareSrv *service.CloudflareService) *LightsailCFJob {
	stats := &LightsailJobStats{0, 0, 0, 0}
	return &LightsailCFJob{conf: conf, apiClient: apiClient, lightsailSrv: lightsailSrv, cloudflareSrv: cloudflareSrv, stats: stats}
}

func (m *LightsailCFJob) Init() error {
	log.Infoln("Monitor is initializing")
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

	log.Infoln("Monitor initialization completed")
	return nil
}

func (m *LightsailCFJob) initInstances() error {
	lightsailClient, err := m.lightsailSrv.GetClient()
	if err != nil {
		return fmt.Errorf("init instances error: %s", err)
	}

	instancesOutput, err := lightsailClient.GetInstances(context.TODO(), &lightsail.GetInstancesInput{PageToken: nil})
	if err != nil {
		return fmt.Errorf("init instances error: %s", err)
	}

	for _, instance := range instancesOutput.Instances {
		if instance.PublicIpAddress != nil {
			m.instances.Store(*instance.PublicIpAddress, instance)
		}
	}
	log.Infof("Get %d VPS instances\n", len(instancesOutput.Instances))
	return nil
}

func (m *LightsailCFJob) initStaticIps() error {
	lightsailClient, err := m.lightsailSrv.GetClient()
	if err != nil {
		return fmt.Errorf("init static ips error: %s", err)
	}

	staticIpsOutput, err := lightsailClient.GetStaticIps(context.TODO(), &lightsail.GetStaticIpsInput{PageToken: nil})
	if err != nil {
		return err
	}

	for _, staticIp := range staticIpsOutput.StaticIps {
		if staticIp.IpAddress != nil {
			m.staticIps.Store(*staticIp.IpAddress, staticIp)
		}
	}

	log.Infof("Get %d static ips\n", len(staticIpsOutput.StaticIps))
	return nil
}

func (m *LightsailCFJob) initDnsRecords() error {
	rootDomain, err := util.GetRootDomain(m.conf.Domain)
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	api, err := m.cloudflareSrv.GetAPI()
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	zones, err := api.ListZones(context.TODO(), rootDomain)
	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	if len(zones) != 1 {
		return fmt.Errorf("init dns records error: %s", "dns zones return value error")
	}

	zoneId := zones[0].ID
	m.dnsZoneId = zoneId
	records, _, err := api.ListDNSRecords(context.TODO(), cloudflare.ZoneIdentifier(zoneId), cloudflare.ListDNSRecordsParams{
		Name: m.conf.Domain,
		Type: "A",
	})

	log.Infof("Get %d DNS records\n", len(records))

	if err != nil {
		return fmt.Errorf("init dns records error: %s", err)
	}

	for _, record := range records {
		m.dnsRecords.Store(record.Content, &record)
	}
	return nil
}

func (m *LightsailCFJob) reloadInstances() error {
	m.instances = sync.Map{}
	return m.initInstances()
}

func (m *LightsailCFJob) reloadStaticIps() error {
	m.staticIps = sync.Map{}
	return m.initStaticIps()
}

func (m *LightsailCFJob) reloadDnsRecords() error {
	m.staticIps = sync.Map{}
	return m.initDnsRecords()
}

func (m *LightsailCFJob) Run() (rerunState bool, err error) {
	m.access.Lock()
	defer m.access.Unlock()
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
	log.Infof("Found %d walled hosts\n", bannedListLen)
	for _, bannedItem := range bannedList {
		log.Infof("current banned host: %s\n", bannedItem)
		instance, ok := m.instances.Load(bannedItem.IP)
		if !ok {
			m.stats.skip++
			log.Errorf("No instance found, %s \n", bannedItem)
			continue
		}
		log.Infof("found instance: %s\n", bannedItem)

		newIp, err := m.processInstance(instance.(types.Instance))
		if err != nil {
			log.Errorf("Process instance failed: %s\n", err)
			m.stats.skip++
			continue
		}

		log.Infof("%s Getting a new ip: %s\n", bannedItem, *newIp)
		pingResult, err := m.testPing(*newIp, bannedItem.Port, defaultPingTryNum)
		if err != nil {
			log.Errorf("Test ping failed: %s\n", err)
			m.stats.skip++
			continue
		}
		log.Infof("Ip %s ping result is %v\n ", *newIp, pingResult)

		if pingResult {
			m.stats.success++
		} else {
			m.stats.fail++
		}

		err = m.changeIp(bannedItem, newIp, pingResult)
		if err != nil {
			log.Errorf("Change ip error: %s\n", err)
			m.stats.skip++
			continue
		}
		log.Infoln("change ip success")
		record, ok := m.dnsRecords.Load(bannedItem.IP)
		api, _ := m.cloudflareSrv.GetAPI()
		if ok {
			if err = api.DeleteDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(m.dnsZoneId), record.(*cloudflare.DNSRecord).ID); err != nil {
				log.Errorf("Delete dns record error: %s\n", err)
				m.stats.skip++
				continue
			}
			log.Infof("delete dns record %s : %s\n", m.conf.Domain, bannedItem.IP)
		}

		_, err = api.CreateDNSRecord(context.TODO(), cloudflare.ZoneIdentifier(m.dnsZoneId), cloudflare.CreateDNSRecordParams{
			Type:    "A",
			Name:    m.conf.Domain,
			Content: *newIp,
			TTL:     60,
		})

		if err != nil {
			log.Errorf("Add dns record  %s : %s error: %s\n", m.conf.Domain, *newIp, err)
			m.stats.skip++
			continue
		}
		log.Infof("Add dns record %s : %s\n", m.conf.Domain, *newIp)

	}

	if m.stats.fail > 0 {
		return true, nil
	}

	return false, nil
}

func (m *LightsailCFJob) testPing(host string, port int, tryNum int) (bool, error) {
	i := 0
	var pingResult bool
	for i < tryNum {
		pingResult, err := m.apiClient.TestPing(host, port)
		i++
		log.Infof("Host %s:%d  ping result is %v \n %d times\n", host, port, pingResult, i)
		if err != nil {
			return false, err
		}
		if pingResult {
			break
		}
		time.Sleep(defaultSleepTime)
	}

	return pingResult, nil
}

func (m *LightsailCFJob) changeIp(bannedItem *api.BannedHostInfo, newIp Ip, pingResult bool) error {
	return m.apiClient.ChangeIP(bannedItem.Type, bannedItem.ID, bannedItem.IP, *newIp, !pingResult)
}

func (m *LightsailCFJob) processInstance(instance types.Instance) (newIP *string, err error) {
	log.Infof("process instance {name: %s, ip:%s}\n", *instance.Name, *instance.PublicIpAddress)
	if *instance.IsStaticIp {
		log.Infof("{name: %s, ip:%s} must be release static ip\n", *instance.Name, *instance.PublicIpAddress)
		return m.processInstanceReleaseStaticIp(instance)
	}
	//Allocate Static Ip
	if m.lenStaticIps() >= service.LightsailMaxStaticIp {
		log.Infof("{name: %s, ip:%s} must be restart to get new IP\n", *instance.Name, *instance.PublicIpAddress)
		return m.processInstanceMustRestart(instance)
	}
	log.Infof("{name: %s, ip:%s} need to allocate a new static IP\n", *instance.Name, *instance.PublicIpAddress)
	return m.processInstanceAllocateStaticIp(instance)
}

func (m *LightsailCFJob) processInstanceReleaseStaticIp(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	staticIp, ok := m.staticIps.Load(*instance.PublicIpAddress)
	if !ok {
		return nil, fmt.Errorf("No static ip found, %s \n", *instance.PublicIpAddress)
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
	return newIp, nil
}

func (m *LightsailCFJob) processInstanceMustRestart(instance types.Instance) (newIp Ip, err error) {
	lightsailClient, _ := m.lightsailSrv.GetClient()
	_, err = lightsailClient.StopInstance(context.TODO(), &lightsail.StopInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}
	log.Infof("{name: %s} is stoping\n", *instance.Name)
	stateName := ""
	for stateName != "stopped" {
		stateOutput, err := lightsailClient.GetInstanceState(context.TODO(), &lightsail.GetInstanceStateInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			return nil, err
		}
		stateName = *stateOutput.State.Name
		log.Infof("{name: %s} state is %s\n", *instance.Name, *stateOutput.State.Name)
		time.Sleep(defaultSleepTime)
	}
	log.Infof("{name: %s} is stopped\n", *instance.Name)
	_, err = lightsailClient.StartInstance(context.TODO(), &lightsail.StartInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}

	log.Infof("{name: %s} is starting\n", *instance.Name)
	for stateName != "running" {
		stateOutput, err := lightsailClient.GetInstanceState(context.TODO(), &lightsail.GetInstanceStateInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			return nil, err
		}
		stateName = *stateOutput.State.Name
		log.Infof("{name: %s, ip:%s} state is %s\n", *instance.Name, *instance.PublicIpAddress, *stateOutput.State.Name)
		time.Sleep(defaultSleepTime)
	}
	log.Infof("{name: %s} is running\n", *instance.Name)

	instanceOutput, err := lightsailClient.GetInstance(context.TODO(), &lightsail.GetInstanceInput{InstanceName: instance.Name})
	if err != nil {
		return nil, err
	}
	newIp = instanceOutput.Instance.PublicIpAddress
	return newIp, nil
}

func (m *LightsailCFJob) lenStaticIps() int {
	i := 0
	m.staticIps.Range(func(k, v interface{}) bool {
		i++
		return true
	})
	return i
}

func (m *LightsailCFJob) lenInstances() int {
	i := 0
	m.instances.Range(func(k, v interface{}) bool {
		i++
		return true
	})
	return i
}

func (m *LightsailCFJob) lenDnsRecords() int {
	i := 0
	m.dnsRecords.Range(func(k, v interface{}) bool {
		i++
		return true
	})
	return i
}
