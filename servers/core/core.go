package core

/*
	Core:
		funckje:
			hostowanie/uruchamianie serwera http
				> lokalna obsługa db tak samo jak inne db
			uruchamianie serwera do komunikacji z innymi serwerami db (ws / ptp)
				> komunikacja pomidzy serwerami github.com/PAW122/TsunamiDB
			uruchamianie serwera do komunikacji z klientami (ws / ptp)
				> komunikacja z klientami github.com/PAW122/TsunamiDB


		moze w przyszłośco:
			auto updaty
			obsługa wielu wersji db
			custom ui

*/

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	config "github.com/PAW122/TsunamiDB/servers/config"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
)

var defaultConfigDir = "./config.json"

func RunCore() {
	debug.Log("Load config")
	config.LoadConfig(defaultConfigDir)

	debug.Log("Run Core")
	config := flag.Bool("config", false, "load config from config.json")
	flag.Parse()

	if *config {
		fmt.Println("load config")
	}

	if len(os.Args) < 2 {
		log.Fatal("Użycie: go run main.go <port> [peer1] [peer2] ...")
	}

	port, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("Niepoprawny port:", err)
	}
	fmt.Println("Starting network manager on port: ", port)

	// Lista znanych peerów (opcjonalna)
	var knownPeers []string
	if len(os.Args) > 2 {
		knownPeers = os.Args[2:]
	}

	networkmanager.StartNetworkManager(port, knownPeers)

	fmt.Println("Starting server on port: ", 5844)
	public_api_v1.RunPublicApi_v1(5844)
}
