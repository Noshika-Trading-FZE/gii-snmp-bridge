package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

type tnApiError struct {
	status int
	prob   string
}

func (e *tnApiError) Error() string {
	return fmt.Sprintf("%s - %d", e.prob, e.status)
}

type Version struct {
	Version uint32 `json:"version"`
}

type VersionResponse struct {
	Error     string `json:"error"`
	UriPrefix string `json:"uri-prefix"`
	Version   uint32 `json:"version"`
	Build     string `json:"build"`
}

type DeviceList struct {
	Deveuis []string `json:"deveuis"`
	Error   string
}

type OwnerInfo struct {
	Owner2Info map[string]OINFO `json:"owner2info"`
	Error      string           `json:"error"`
}

type OINFO struct {
	Ownerid6 string   `json:"ownerid6"`
	Apisuri  string   `json:"apisuri"`
	Wsuri    string   `json:"wsuri"`
	Tokens   []string `json:"tokens"`
}

var client = &http.Client{Timeout: 20 * time.Second}

func getJson(method string, url string, request interface{}, target interface{}) error {
	b := new(bytes.Buffer)
	if request != nil {
		json.NewEncoder(b).Encode(request)
	}

	req, _ := http.NewRequest(method, config.Tracknet.Host+url, b)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", config.Tracknet.Token)

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	bodyString := string(bodyBytes)

	log.Debugf("Response status: %d\n", res.StatusCode)
	log.Debugf("Response body: \"%s\"\n", bodyString)

	if res.StatusCode == http.StatusOK {
		return json.Unmarshal([]byte(bodyString), target)
	}

	return &tnApiError{res.StatusCode, bodyString}
}

// Returns TN version
func getVersion() (*VersionResponse, error) {
	var versionResp VersionResponse
	err := getJson("POST", "", Version{Version: 1}, &versionResp)

	return &versionResp, err
}

// func getDeviceList() (*DeviceList, error) {
// 	var devList DeviceList
// 	err := getJson("GET", "device/list", nil, &devList)

// 	return &devList, err
// }

func getOwnerList() (*OwnerInfo, error) {
	var oinfo OwnerInfo
	err := getJson("GET", "owner/info", nil, &oinfo)

	return &oinfo, err
}
