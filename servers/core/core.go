package core

/*
	Core:
		funkcje:
			hostowanie/uruchamianie serwera http
				> lokalna obsluga db tak samo jak inne db
			uruchamianie serwera do komunikacji z innymi serwerami db (ws / ptp)
				> komunikacja pomiedzy serwerami github.com/PAW122/TsunamiDB
			uruchamianie serwera do komunikacji z klientami (ws / ptp)
				> komunikacja z klientami github.com/PAW122/TsunamiDB

		moze w przyszlosci:
			auto updaty
			obsluga wielu wersji db
			custom ui
*/

import (
	"fmt"
	"log"
	"os"

	config "github.com/PAW122/TsunamiDB/servers/config"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
	ui "github.com/PAW122/TsunamiDB/servers/ui"
)

var defaultConfigDir = "./config.json"

func RunCore() {
	debug.Log("Load config")
	config.LoadConfig(defaultConfigDir)

	debug.Log("Run Core")

	opts, err := parseRunOptions(os.Args[1:])
	if err != nil {
		log.Fatalf("Blad argumentow: %v\n%s", err, usageMessage)
	}

	if opts.loadConfig {
		fmt.Println("load config")
	}

	fmt.Println("Starting network manager on port:", opts.corePort)

	if opts.uiPort > 0 {
		go func() {
			if err := ui.RunAdminUI(opts.uiPort); err != nil {
				log.Printf("UI server stopped: %v", err)
			}
		}()
	}

	networkmanager.StartNetworkManager(opts.corePort, opts.peers)

	fmt.Println("Starting sub Sever on port:", 5845)
	go subServer.StartWSServer("5845")

	fmt.Println("Starting server on port:", 5844)
	public_api_v1.RunPublicApi_v1(5844)
}
