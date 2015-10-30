package registrator

import (
	"flag"
	log "github.com/Sirupsen/logrus"
	"time"

	"github.com/contiv/netplugin/registrator/bridge"
)

// QuitCh maintains the status of netplugin
var QuitCh chan struct{}

var retryAttempts = flag.Int("retry-attempts", 0, "Max retry attempts to establish a connection with the backend. Use -1 for infinite retries")
var retryInterval = flag.Int("retry-interval", 2000, "Interval (in millisecond) between retry-attempts.")

func assert(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// InitRegistrator creates a new bridge to Service registrator
func InitRegistrator(bridgeType string, bridgeCfg ...bridge.Config) (*bridge.Bridge, error) {

	log.Printf("Initing registrator from Dockplugin")
	var bConfig bridge.Config

	defaultBridgeConfig := bridge.DefaultBridgeConfig()
	if len(bridgeCfg) == 0 {
		bConfig = defaultBridgeConfig
	} else {
		bConfig = bridgeCfg[0]
		if bConfig.HostIP != "" {
			log.Println("Forcing host IP to", bConfig.HostIP)
		}

		if (bConfig.RefreshTTL == 0 && bConfig.RefreshInterval > 0) ||
			(bConfig.RefreshTTL > 0 && bConfig.RefreshInterval == 0) {
			log.WithFields(log.Fields{
				"RefreshTTL":      bConfig.RefreshTTL,
				"RefreshInterval": bConfig.RefreshInterval,
			}).Warn("RefreshTTL and RefreshInterval ",
				"must be specified together or not at all. ",
				"Setting to default values (",
				defaultBridgeConfig.RefreshTTL,
				",", defaultBridgeConfig.RefreshInterval,
				") and continuing")
			bConfig.RefreshTTL = defaultBridgeConfig.RefreshTTL
			bConfig.RefreshInterval = defaultBridgeConfig.RefreshInterval
		} else if bConfig.RefreshTTL > 0 &&
			bConfig.RefreshTTL <= bConfig.RefreshInterval {
			log.WithFields(log.Fields{
				"RefreshTTL":      bConfig.RefreshTTL,
				"RefreshInterval": bConfig.RefreshInterval,
			}).Warn("RefreshTTL must be greater than RefreshInterval",
				". Setting to default values (",
				defaultBridgeConfig.RefreshTTL,
				",", defaultBridgeConfig.RefreshInterval,
				") and continuing")
			bConfig.RefreshTTL = defaultBridgeConfig.RefreshTTL
			bConfig.RefreshInterval = defaultBridgeConfig.RefreshInterval
		}

		if bConfig.DeregisterCheck != "always" &&
			bConfig.DeregisterCheck != "on-success" {
			log.WithFields(log.Fields{
				"DegisterCheck": bConfig.DeregisterCheck,
			}).Warn("Deregister must be \"always\" or ",
				"\"on-success\". Setting to default value of ",
				defaultBridgeConfig.DeregisterCheck,
				" and continuing")
			bConfig.DeregisterCheck = defaultBridgeConfig.DeregisterCheck
		}
	}

	log.Info("Creating a new bridge: ", bridgeType)

	b, err := bridge.New(bridgeType, bConfig)

	if err != nil {
		log.WithFields(log.Fields{
			"Error": err,
		}).Error("Bridge creation errored out. ",
			"Service registration will not work.")
		return nil, err
	}

	// TODO: Add proper checks for retryInterval, retryAttempts
	attempt := 0
	for *retryAttempts == -1 || attempt <= *retryAttempts {
		log.Printf("Connecting to backend (%v/%v)", attempt, *retryAttempts)

		err = b.Ping()
		if err == nil {
			break
		}

		if err != nil && attempt == *retryAttempts {
			assert(err)
		}

		time.Sleep(time.Duration(*retryInterval) * time.Millisecond)
		attempt++
	}

	// Start the TTL refresh timer
	if bConfig.RefreshInterval > 0 {
		ticker := time.NewTicker(time.Duration(bConfig.RefreshInterval) * time.Second)
		go func() {
			for {
				select {
				case <-ticker.C:
					b.Refresh()
				case <-QuitCh:
					log.Infof("Quit registrator")
					ticker.Stop()
					return
				}
			}
		}()
	}

	return b, nil
}
