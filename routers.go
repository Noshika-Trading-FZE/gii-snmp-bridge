package main

import (
	"errors"
	"net/url"
	"time"

	"github.com/go-resty/resty"
	log "github.com/sirupsen/logrus"
)

type LNSRouterPoller struct {
	Url   string
	Owner string
	Token string
	// MessageID int64

	PollCycle int
	Routers   []TCTRouterStatusObject
}

type TCTRoutersListResult struct {
	Routers []int64
}

type TCTRouterStatusObject struct {
	Connected    bool
	Connection   string
	Managed      bool
	Name         string
	HwSpec       string
	RouterID     int64  `json:"routerid"`
	RouterID6    string `json:"routerid6"`
	MacADDR      string `json:"macaddr"`
	CntUp        int64  `json:"cnt_up"`
	CntDown      int64  `json:"cnt_down"`
	Firmware     string
	LastUptime   float64 `json:"last_uptime"`
	LastDonwtime float64 `json:"last_dntime"`
	LastConnect  float64 `json:"last_connect"`
	Gpsloc       []float64

	dirty bool `json:"-"`
}

type TCTRouterStatusResponse struct {
	Routers []TCTRouterStatusObject `json:"routers"`
	Error   string                  `json:"error"`
}

func NewLNSRouterPoller(uri string, owner string, token string, pollCycle int) (*LNSRouterPoller, error) {
	_, err := url.ParseRequestURI(uri)
	if err != nil {
		return nil, err
	}

	// resty.SetDebug(true)
	resty.SetCloseConnection(true)

	return &LNSRouterPoller{
		Url:       uri,
		Owner:     owner,
		Token:     token,
		PollCycle: pollCycle,
	}, nil
}

func (t *LNSRouterPoller) Run() {
	if t.PollCycle <= 0 {
		return
	}

	for {
		err := t.loadRouters()
		if err != nil {
			log.Error(err)
		}

		for i, r := range t.Routers {
			// log.Debugf("%#v", r)
			if r.dirty {
				if snmp != nil {
					snmp.Stream <- snmp.TrapRouter(r.Name, !r.Connected)
				}
				t.Routers[i].dirty = false
			}
		}

		time.Sleep(time.Duration(t.PollCycle) * time.Second)
	}
}

func (t *LNSRouterPoller) loadRouters() error {
	routersID, err := t.getRoutersList()
	if err != nil {
		return err
	}

	routers, err := t.getRoutersStatus(routersID)
	if err != nil {
		return err
	}

	result := []TCTRouterStatusObject{}

	for _, rNew := range *routers {
		new := true
		for _, rCurr := range t.Routers {
			if rCurr.MacADDR == rNew.MacADDR {
				new = false
				if rCurr.Connected != rNew.Connected {
					rNew.dirty = true
					break
				}
			}
		}
		if new {
			rNew.dirty = true
		}
		result = append(result, rNew)
	}

	t.Routers = result

	return err
}

func (t *LNSRouterPoller) getRoutersList() ([]int64, error) {
	var result TCTRoutersListResult

	_, err := resty.R().
		SetHeader("Authorization", t.Token).
		SetResult(&result).
		Get(t.Url + "router/list")

	if err != nil {
		return []int64{}, err
	}

	return result.Routers, nil
}

func (t *LNSRouterPoller) getRoutersStatus(routers []int64) (*[]TCTRouterStatusObject, error) {
	var result TCTRouterStatusResponse

	request := routers
	//144255936773372160
	//144255936773372160
	//[]string{"1"} //fmt.Sprintf("[%d]", ID)

	// log.Debugf("%+v", request)

	_, err := resty.R().
		SetHeader("Authorization", t.Token).
		SetBody(&request).
		SetResult(&result).
		Post(t.Url + "router/status")

	if err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, errors.New(result.Error)
	}

	if len(result.Routers) == 0 {
		return nil, errors.New("routers list is empty")
	}

	// t.Routers = result.Routers
	return &(result.Routers), nil
}
