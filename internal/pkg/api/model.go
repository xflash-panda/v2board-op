package api

import "fmt"

// API is the interface for different panel's api.*/

type API interface {
	QueryBannedList(tags []string) (bannedList []*BannedHostInfo, err error)
	TestPing(host string, port int) (result PingResult, err error)
	ChangeIP(nodeType string, id int, sourceIp string, targetIp string, walledStatus bool) (err error)
	//	GetUserList() (userList []*UserInfo, err error)
	//	ReportUserTraffic(userTraffic []*UserTraffic) (err error)
	//	Describe() *ClientInfo
	Debug()
}

type BannedHostInfo struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type RepQueryBannedList struct {
	Data []*BannedHostInfo `json:"data"`
}

type PingResult bool
type ChangeIpResult bool

type RepTestPing struct {
	Data PingResult `json:"data"`
}

type RepChangIpResult struct {
	Data PingResult `json:"data"`
}

func (i *BannedHostInfo) String() string {
	return fmt.Sprintf("{type: %s id: %d ip: %s port: %d}", i.Type, i.ID, i.IP, i.Port)
}
