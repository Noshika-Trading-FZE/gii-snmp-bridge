package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"thingularity/devices"
	model "thingularity/papi/models"
	"time"

	"github.com/krylovsk/gosenml"
	log "github.com/sirupsen/logrus"
)

type TCIONodes struct {
	Error    string   `json:"error"`
	Owner    string   `json:"owner"`
	APPXList []TCIOEP `json:"appx_list"`
	Version  int      `json:"version"`
	Release  int      `json:"release"`
}

type TCIOEP struct {
	APPXId  string `json:"appxid"`
	Uri     string `json:"uri"`
	Version int    `json:"version"`
}

type TCIOMessage struct {
	Msgtype string `json:"msgtype"`
}

type TCIOUpdf struct {
	Msgtype    string  `json:"msgtype"`
	DevEui     string  `json:"DevEui"`
	UpId       int64   `json:"upid"`
	SessID     int64   `json:"SessID"`
	FCntUp     int64   `json:"FCntUp"`
	FPort      int     `json:"FPort"`
	FRMPayload string  `json:"FRMPayload"`
	DR         int     `json:"DR"`
	Freq       int     `json:"Freq"`
	Region     string  `json:"region"`
	DClass     string  `json:"dClass"`
	ArrTime    float64 `json:"ArrTime"`
	Confirm    bool    `json:"confirm"`
}

type TCIOUpinfo struct {
	Msgtype string `json:"msgtype"`
	DevEui  string `json:"DevEui"`
	DR      int    `json:"DR"`
	FPort   int    `json:"FPort"`
	Freq    int    `json:"Freq"`
	FCntUp  int64  `json:"FCntUp"`
	DClass  string `json:"dClass"`
	Region  string `json:"region"`
	UpId    int64  `json:"upid"`
	UpInfo  []struct {
		RouterId int64   `json:"routerid"`
		MuxId    int64   `json:"muxid"`
		RSSI     float64 `json:"rssi"`
		SNR      float64 `json:"snr"`
		ArrTime  float64 `json:"ArrTime"`
	} `json:"upinfo"`
}

type TCIODndf struct {
	Msgtype    string `json:"msgtype"`
	MsgId      byte   `json:"MsgId"`
	FPort      int    `json:"FPort"`
	FRMPayload string `json:"FRMPayload"`
	DevEui     string `json:"DevEui"`
	Confirm    bool   `json:"confirm"`
}

type TCIODnacked struct {
	Msgtype string `json:"msgtype"`
	MsgId   byte   `json:"MsgId"`
	DevEui  string `json:"DevEui"`
	UpId    int64  `json:"upid"`
}

type LorIOTMessage struct {
	Cmd       string `json:"cmd"`
	EUI       string `json:"EUI"`
	TimeStamp int64  `json:"ts"`
	// RSSI      float64 `json:"rssi"`
	// SNR       float64 `json:"snr"`
	Port int    `json:"port"`
	Data string `json:"data"`
	DR   int    `json:"dr"`
	FCnt int64  `json:"fcnt"`
	Freq int    `json:"freq"`
}

type LorIOTDownMessage struct {
	Cmd       string `json:"cmd"` // "tx"
	EUI       string `json:"EUI"`
	Port      int    `json:"port"`
	Data      string `json:"data"`
	Confirmed bool   `json:"confirmed"`
}

var msgid byte = 0
var appx []*WssClient

func tcioGetNextMsgId() byte {
	msgid = msgid + 1
	return msgid
}

func readTCIOs(response []byte, target interface{}) error {
	return json.Unmarshal(response, target)
}

func tcioMessageHandler(c *WssClient, received []byte) {
	log.Debugf("WebSocket.RECEIVED: %s\n", received)
	var nodes TCIONodes

	err := readTCIOs(received, &nodes)
	if err != nil {
		log.Fatal(err)
	}

	// log.Debugf("%+v", nodes)

	for _, ep := range nodes.APPXList {
		log.Debugf("TCIO connection points: %s", ep.Uri)
		processTCIOconnection(ep)
	}
}

func processTCIOconnection(ep TCIOEP) {
	tcio, err := NewWssClient(ep.Uri, true, tcioDataHandler)
	if err != nil {
		log.Fatalf("NewWssClient: %s", err)
	}

	appx = append(appx, tcio)

	go tcio.writePump()
	go tcio.readPump()
}

func tcioDataHandler(c *WssClient, received []byte) {
	log.Debugf("WebSocket(%s).RECEIVED: %s\n", c.uri, received)

	var m TCIOMessage
	err := readTCIOs(received, &m)
	if err != nil {
		log.Fatal(err)
	}

	switch m.Msgtype {
	case "updf":
		tcioUpdf(received)
	case "upinfo":
		tcioUpinfo(received)
	case "dnacked":
		tcioDnacked(received)
	default:
		log.Debugf("Unsupported message type %s", m.Msgtype)
	}
}

type TheMessage struct {
	Placeholder model.Placeholder
	Device      model.PhysicalDevice
	Senml       interface{}
}

func tcioUpdf(received []byte) {
	var (
		m            TCIOUpdf
		sensorDevice devices.Device
	)

	err := readTCIOs(received, &m)
	if err != nil {
		log.Fatal(err)
	}

	EUI := strings.Replace(m.DevEui, "-", "", -1)
	pd, err := getPhysicalDeviceByEUI(EUI)
	if err != nil {
		log.Error(err)
		return
	}

	if !pd.Enabled {
		log.Infof("Device %s, EUI: %s is disabled. Skipping.", pd.Id, EUI)

		pd.HwState.LastSeen = time.Unix(int64(m.ArrTime), 0)
		pd.HwState.FCntUp = m.FCntUp
		pd.HwState.State = "Active"
		pd.HwState.Confirm = m.Confirm
		pd.HwState.UpCounter = pd.HwState.UpCounter + 1

		err = updatePhysicalDevice(pd.Id, pd)
		if err != nil {
			log.Error(err)
		}

		return
	}

	if len(m.FRMPayload) == 0 {
		log.Errorf("Payload from %s is empty.", m.DevEui)
		return
	}

	data, err := hex.DecodeString(m.FRMPayload)
	if err != nil {
		log.Error(err)
		return
	}

	sensorDevice = devices.GetDriver(pd.Model)
	if sensorDevice == nil {
		log.Errorf("Unknown model for device EUI=%s", EUI)
	}

	sensorDevice.SetEUI(EUI)

	deviceDecoder := devices.NewDeviceDecoder(sensorDevice)
	err = deviceDecoder.DecodeRaw(&pd, data, time.Unix(int64(m.ArrTime), 0), byte(m.FPort))
	if err != nil {
		log.Error(err)
		return
	}
	jsonString, err := deviceDecoder.EncodeSenml()
	if err != nil {
		log.Errorf("Encoding SenML failed for %s: %v", pd.LoraWAN.EUI, err)
		return
	}

	pd.UpdateMeasurements(deviceDecoder.GetSenml())

	switch pd.Model {
	// lw360
	case "5BD3C1B4-F6C1-48DF-B628-F4409E6CD478":
		// add to reset queue
		alarm, ok := deviceDecoder.GetMeasurement("GPS_ALARM_TYPE")
		if ok {
			if byte(*alarm.Value) > 0 {
				//
				var resetStatus float64 = 0

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Second * 30),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:  "GPS_ALARM_TYPE",
								Value: &resetStatus,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}
	//netvox R718DB
	case "08E6CB75-6ADB-4D9A-A0D3-2BA9C88E6B32":
		// add to reset queue
		status, ok := deviceDecoder.GetMeasurement("STATUS")
		if ok {
			if byte(*status.Value) > 0 {
				//
				var resetStatus float64 = 0

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Second * 10),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:  "STATUS",
								Value: &resetStatus,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}

		//netvox R718DB 2min
	case "C6C8BCB3-C93B-4F51-81A0-D7059F3BBBB8":
		status, ok := deviceDecoder.GetMeasurement("STATUS")
		if ok {
			if byte(*status.Value) > 0 {
				//
				var resetStatus float64 = 0

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Minute * 2),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:  "STATUS",
								Value: &resetStatus,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}

		// netvox R312A 5sec
	case "63707974-0543-4EAA-AAC5-AD26F18AFECB":
		status, ok := deviceDecoder.GetMeasurement("DOOR_BELL")
		if ok {
			if byte(*status.Value) > 0 {
				//
				var resetStatus float64 = 0

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Second * 5),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:  "DOOR_BELL",
								Value: &resetStatus,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}

		// ELT Waterleak 2min
		// "ABA9BF90-05AC-4A8C-80B9-FC408D602289"
	case "ABA9BF90-05AC-4A8C-80B9-FC408D602289":
		// add to reset queue
		status, ok := deviceDecoder.GetMeasurement("WATERLEAK_STATUS")
		if ok {
			if byte(*status.Value) > 0 {
				//
				var (
					resetStatus1 float64 = 0
					resetStatus2 float64 = 0
				)

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Minute * 2),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:  "WATERLEAK",
								Value: &resetStatus1,
							},
							{
								Name:  "WATERLEAK_STATUS",
								Value: &resetStatus2,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}

	// Lansitec SAT tracker
	case "5B142EEC-79E9-428F-A810-1858D4EA5868":
		// add to reset queue
		alarm, ok := deviceDecoder.GetMeasurement("ALARM")
		if ok {
			if *alarm.BooleanValue {
				//
				resetStatus := false

				resetQueue[pd.LoraWAN.EUI] = ResetMessage{
					ResetTime: time.Now().Add(time.Second * 30),
					Senml: gosenml.Message{
						Version: 1.0,
						Entries: []gosenml.Entry{
							{
								Name:         "ALARM",
								BooleanValue: &resetStatus,
							},
						},
						BaseName: "urn:dev:eui:" + pd.LoraWAN.EUI,
						BaseTime: deviceDecoder.GetSenml().BaseTime,
					},
				}
			}
		}

		msgid, ok := deviceDecoder.GetMeasurement("MSGID")
		if ok {
			val := *msgid.Value
			cmd, port, err := sensorDevice.EncodeCommand("SEND_ACK", &devices.CommandParameters{
				"MSGID": val,
			})
			if err == nil {
				log.Error(err)
			} else {
				msg := LorIOTDownMessage{
					EUI:       pd.LoraWAN.EUI,
					Port:      int(port),
					Confirmed: false,
					Data:      cmd,
				}

				tcioDndf(msg)
			}
		}
	}

	nc.Publish(pubTopic+EUI+".senml", []byte(jsonString))

	if bat, ok := deviceDecoder.GetBatteryLevel(); ok {
		pd.HwState.Battery = *bat.Value
	}

	if dc, ok := deviceDecoder.GetDCPower(); ok {
		pd.HwState.DCPower = *dc.BooleanValue
	}

	pd.HwState.LastSeen = time.Unix(int64(m.ArrTime), 0)
	pd.HwState.FCntUp = m.FCntUp
	pd.HwState.State = "Active"
	pd.HwState.Confirm = m.Confirm
	pd.HwState.UpCounter = pd.HwState.UpCounter + 1

	err = updatePhysicalDevice(pd.Id, pd)
	if err != nil {
		log.Error(err)
	}

	// if err = updatePhysicalDeviceHW(pd.Id, pd.HwState); err != nil {
	// 	log.Error(err)
	// }

	placeholder, err := getPlaceholder(pd.PlaceholderId)
	if err != nil {
		log.Error(err)
	}

	placeholderJson, _ := json.Marshal(placeholder)
	deviceJson, _ := json.Marshal(pd)
	theMessage := fmt.Sprintf("{\"placeholder\":%s,\"device\":%s,\"senml\":%s}", string(placeholderJson), string(deviceJson), jsonString)

	log.Debugf("Sending combined message: %s", theMessage)

	nc.Publish(combinedTopic, []byte(theMessage))

	// FlushTimeout specifies a timeout value as well.
	err = nc.FlushTimeout(2 * time.Second)
	if err != nil {
		log.WithFields(log.Fields{"module": "tcio", "type": "nats"}).Fatal("Flushed timed out!")
	} else {
		log.WithFields(log.Fields{"module": "tcio", "type": "nats"}).Debugf("Sensor state %s sent to %s\n", jsonString, EUI)
	}
}

func tcioUpinfo(received []byte) {
	var (
		m            TCIOUpinfo
		sensorDevice devices.Device
	)

	err := readTCIOs(received, &m)
	if err != nil {
		log.Error(err)
		return
	}

	EUI := strings.Replace(m.DevEui, "-", "", -1)
	pd, err := getPhysicalDeviceByEUI(EUI)
	if err != nil {
		log.Error(err)
		return
	}
	m.DevEui = EUI

	// find strongest RSSI
	var (
		rssi  float64 = -1000
		snr   float64
		dirty bool
	)

	for _, upinfo := range m.UpInfo {
		if upinfo.RSSI > rssi {
			rssi = upinfo.RSSI
			snr = upinfo.SNR
			dirty = true
		}
	}

	if dirty {
		pd.HwState.RSSI = rssi
		pd.HwState.SNR = snr

		if err = updatePhysicalDeviceHW(pd.Id, pd.HwState); err != nil {
			log.Error(err)
		}

		sensorDevice = devices.GetDriver(pd.Model)
		if sensorDevice == nil {
			log.Errorf("Unknown model for device EUI=%s", EUI)
		}

		sensorDevice.SetEUI(EUI)

		if _, ok := sensorDevice.GetAbilities()["RSSI"]; ok {
			deviceDecoder := devices.NewDeviceDecoder(sensorDevice)
			err := deviceDecoder.CreateRSSIinfo(rssi, snr)
			if err != nil {
				log.Error(err)
				return
			}

			jsonString, err := deviceDecoder.EncodeSenml()
			if err != nil {
				log.Error(err)
				return
			}

			log.Debugf("Send rssi measurements %s", jsonString)
			nc.Publish(pubTopic+EUI+".senml", []byte(jsonString))
		}
	}

	placeholder, err := getPlaceholder(pd.PlaceholderId)
	if err != nil {
		log.Error(err)
	}

	placeholderJson, _ := json.Marshal(placeholder)
	deviceJson, _ := json.Marshal(pd)
	rssiJson, _ := json.Marshal(m)
	theMessage := fmt.Sprintf("{\"placeholder\":%s,\"device\":%s,\"rssi\":%s}", string(placeholderJson), string(deviceJson), string(rssiJson))

	log.Debugf("Sending combined message: %s", theMessage)

	nc.Publish(upinfoTopic, []byte(theMessage))

	// FlushTimeout specifies a timeout value as well.
	err = nc.FlushTimeout(2 * time.Second)
	if err != nil {
		log.WithFields(log.Fields{"module": "tcio", "type": "nats"}).Fatal("Flushed timed out!")
	} else {
		log.WithFields(log.Fields{"module": "tcio", "type": "nats"}).Debugf("Sensor RSSI info %s sent to %s\n", string(rssiJson), EUI)
	}
}

func tcioDnacked(received []byte) {
	var (
		m TCIODnacked
	)

	err := readTCIOs(received, &m)
	if err != nil {
		log.Error(err)
		return
	}

	EUI := strings.Replace(m.DevEui, "-", "", -1)
	pd, err := getPhysicalDeviceByEUI(EUI)
	if err != nil {
		log.Error(err)
		return
	}
	// m.DevEui = EUI

	// ap.Add("Tnbridge", "CONFIRMED", "true")
	sensorDevice := devices.GetDriver(pd.Model)
	if sensorDevice == nil {
		log.Errorf("Unknown model for device EUI=%s", EUI)
	}

	sensorDevice.SetEUI(EUI)

	deviceDecoder := devices.NewDeviceDecoder(sensorDevice)
	deviceDecoder.AddDnacked()

	jsonString, err := deviceDecoder.EncodeSenml()
	if err != nil {
		log.Error(err)
		return
	}

	log.Debugf("Send DNACKED measurements %s", jsonString)
	nc.Publish(pubTopic+EUI+".senml", []byte(jsonString))

	placeholder, err := getPlaceholder(pd.PlaceholderId)
	if err != nil {
		log.Error(err)
	}

	placeholderJson, _ := json.Marshal(placeholder)
	deviceJson, _ := json.Marshal(pd)
	theMessage := fmt.Sprintf("{\"placeholder\":%s,\"device\":%s,\"senml\":%s}", string(placeholderJson), string(deviceJson), jsonString)

	log.Debugf("Sending DNACKED combined message: %s", theMessage)

	nc.Publish(combinedTopic, []byte(theMessage))

}

// generic to convert LoraEUI to TC format "XX-XX-XX-..."
func insertRunes(s string, interval int, sep rune) string {
	var buffer bytes.Buffer
	before := interval - 1
	last := len(s) - 1
	for i, char := range s {
		buffer.WriteRune(char)
		if i%interval == before && i != last {
			buffer.WriteRune(sep)
		}
	}
	return buffer.String()
}

// Send loriot packet as downlink message to TC
func tcioDndf(m LorIOTDownMessage) {
	var o TCIODndf

	o.Msgtype = "dndf"
	o.MsgId = tcioGetNextMsgId()
	o.FPort = m.Port
	o.Confirm = m.Confirmed
	o.FRMPayload = m.Data
	o.DevEui = insertRunes(m.EUI, 2, '-')

	data, err := json.Marshal(o)
	// log.Debugf("%s", string(data))
	if err != nil {
		log.Error(err)
		return
	}

	if len(appx) > 0 {
		appx[0].doSend(string(data))
	}
}
