package main

import (
	// "encoding/hex"
	// "encoding/json"
	// "fmt"
	// "github.com/koding/multiconfig"
	// "os"
	// "os/signal"
	"thingularity/papi/db"

	"github.com/krylovsk/gosenml"
	nats "github.com/nats-io/go-nats"
	log "github.com/sirupsen/logrus"

	// "regexp"
	"crypto/tls"
	"net/http"

	// "syscall"
	"time"
)

var (
	config        *Config
	pubTopic      string
	combinedTopic string
	upinfoTopic   string
	nc            *nats.Conn
	snmp          *SNMP
	routerPoller  *LNSRouterPoller
)

type ResetMessage struct {
	ResetTime time.Time
	Senml     gosenml.Message
}

var resetQueue map[string]ResetMessage

func main() {
	// var err error
	log.Info("Thingularity :: TrackCentral bridge")

	// // Configuration change signal
	// sighup := make(chan os.Signal, 1)
	// signal.Notify(sighup, syscall.SIGHUP)
	//
	// // Termination signal
	// sigterm := make(chan os.Signal, 1)
	// signal.Notify(sigterm, os.Interrupt)
	// signal.Notify(sigterm, syscall.SIGTERM)

	config = loadConfig()

	if config.Logging.Verbose {
		log.SetLevel(log.DebugLevel)
		log.Debugf("%v", config)
	}

	resetQueue = make(map[string]ResetMessage)

	// // SIGHUP handler
	// go func() {
	// 	for range sighup {
	// 		log.Fatal("SIGHUP: Reloading configuration. We have to restart service.")
	// 	}
	// }()
	//
	// // SIGTERM handler
	// go func() {
	// 	<-sigterm
	// 	log.Fatal("bridge killed !")
	// }()

	// setup default insecure connections for https, if needed
	if config.AllowInsecure {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		log.Warn("HTTPS sets to allow insecure connections! Please check the configuration")
	}

	db.ConnectToDB(config.Database.Host, config.Database.Dataset)
	defer db.CloseDB()

	if config.SNMP.Enabled {
		log.Infof("SNMP module enabled, server is set to %s:%d", config.SNMP.Host, config.SNMP.Port)
		snmp = NewSNMP(config.SNMP.Host, config.SNMP.Port)
		go snmp.Run()

		var err error
		routerPoller, err = NewLNSRouterPoller(config.Tracknet.Host, config.Tracknet.Owner, config.Tracknet.Token, config.SNMP.RoutersPollCycle)
		if err != nil {
			log.Error(err)
		} else {
			go routerPoller.Run()
		}
		// time.Sleep(1 * time.Second)
	}

	ver, err := getVersion()

	if err != nil {
		log.Fatal(err)
	}

	if snmp != nil {
		snmp.Stream <- snmp.TrapLNS(false)
	}

	log.Infof("Connected to TCT, build: %s", ver.Build)

	ownerList, err := getOwnerList()
	if err != nil {
		log.Fatal(err)
	}

	for key, value := range ownerList.Owner2Info {
		log.Debugf("Owner %s, %s, %s, keys: %v", key, value.Apisuri, value.Wsuri, value.Tokens)
	}

	o, ok := ownerList.Owner2Info[config.Tracknet.Owner]
	if !ok {
		log.Fatalf("Owner %s not found in TCT configuration", config.Tracknet.Owner)
	}

	tcio, err := NewWssClient(o.Wsuri, false, tcioMessageHandler)
	if err != nil {
		log.Fatalf("TCIO Client: %s", err)
	}

	go tcio.writePump()
	go tcio.readPump()

	connMessage := "{\"owner\": \"" + config.Tracknet.Owner + "\"}"
	tcio.doSend(connMessage)

	pubTopic = "devices.physical."
	combinedTopic = "messages.uplink"
	upinfoTopic = "upinfo.uplink"
	subTopic := "bridge.control"
	nc, err = nats.Connect(config.Nats.Url)
	if err != nil {
		log.Fatal(err)
	}
	nc.SetDisconnectHandler(func(c *nats.Conn) {
		log.WithFields(log.Fields{"module": "tcio", "type": "nats"}).Panic("Connection lost")
	})

	choke := make(chan *nats.Msg, 64)
	nc.ChanSubscribe(subTopic, choke)

	resetC := time.NewTicker(time.Second).C

	for {
		select {
		case incoming := <-choke:
			message := string(incoming.Data)

			log.WithFields(log.Fields{"module": "nats", "topic": incoming.Subject}).Debug(message)
			// log.Debugf("RECEIVED TOPIC: %s MESSAGE: %s\n", incoming.Subject, message)

			switch message {
			case "reload":
				log.Info("NATS CMD: Reloading configuration")
				// reload()
			}

		case <-resetC:
			//
			for key, rm := range resetQueue {
				now := time.Now()
				if rm.ResetTime.Unix() < now.Unix() {
					enc := gosenml.NewJSONEncoder()
					rMessage, err := enc.EncodeMessage(&rm.Senml)
					if err != nil {
						log.Error(err)
						continue
					}

					nc.Publish("devices.physical."+key+".senml", rMessage)
					log.Debugf("Fake reset message sent from %s, payload = %s", key, string(rMessage))

					delete(resetQueue, key)
				}
			}

		}
	}
}
