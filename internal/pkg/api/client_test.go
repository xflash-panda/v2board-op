package api

import "testing"

func CreateClient() *Client {
	apiConfig := &Config{
		APIHost: "http://127.0.0.1:8080",
		Token:   "123456789123456789",
	}
	client := New(apiConfig)
	return client
}

func TestQueryBannedList(t *testing.T) {
	client := CreateClient()
	bannedList, err := client.QueryBannedList([]string{"lightsail_cf"})
	if err != nil {
		t.Error(err)
	}
	t.Log(bannedList)
}

func TestTestPing(t *testing.T) {
	client := CreateClient()
	bannedList, err := client.TestPing("1.1.1.1", 443)
	if err != nil {
		t.Error(err)
	}
	t.Log(bannedList)
}

func TestChangeIp(t *testing.T) {
	client := CreateClient()
	err := client.ChangeIP("trojan", 32, "52.196.160.249", "52.196.160.249", true)
	if err != nil {
		t.Error(err)
		t.Fail()
	}
}
