package main

import (
	"github.com/koding/multiconfig"
	log "github.com/sirupsen/logrus"
)

const CONFIG_FILE = "bridge.toml"

// Service configuration. Initialize it from config.toml
type Config struct {
	Tracknet struct {
		Host  string `required:"true"`
		Token string `required:"true"`
		Owner string `required:"true"`
	}
	Nats struct {
		Url string `required:"true"`
	}
	Database struct {
		Host    string `required:"true"`
		Dataset string `required:"true"`
	}
	Logging struct {
		Verbose bool `default:"true"`
	}
	AllowInsecure bool `default:"false"`

	SNMP struct {
		Enabled          bool
		Host             string
		Port             int
		RoutersPollCycle int
		// Routers []struct {
		// 	ID   int64
		// 	Name string
		// }
	}
}

func loadConfig() *Config {
	// Load config file
	m := multiconfig.NewWithPath(CONFIG_FILE)
	config := new(Config)

	err := m.Load(config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}
