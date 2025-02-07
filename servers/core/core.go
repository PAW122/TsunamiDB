package core

/*
	Core:
		funckje:
			hostowanie/uruchamianie serwera http
				> lokalna obsługa db tak samo jak inne db
			uruchamianie serwera do komunikacji z innymi serwerami db (ws / ptp)
				> komunikacja pomidzy serwerami TsunamiDB
			uruchamianie serwera do komunikacji z klientami (ws / ptp)
				> komunikacja z klientami TsunamiDB


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

	networkmanager "TsunamiDB/servers/network-manager"
	public_api_v1 "TsunamiDB/servers/public-api/v1"
)

func RunCore() {
	config := flag.Bool("config", false, "load config from config.json")
	flag.Parse()

	if *config {
		fmt.Println("load config")
	}

	fmt.Println("Starting network manager on port: ", 5845)
	if len(os.Args) < 2 {
		log.Fatal("Użycie: go run main.go <port> [peer1] [peer2] ...")
	}

	port, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal("Niepoprawny port:", err)
	}

	// Lista znanych peerów (opcjonalna)
	var knownPeers []string
	if len(os.Args) > 2 {
		knownPeers = os.Args[2:]
	}

	networkmanager.StartNetworkManager(port, knownPeers)

	fmt.Println("Starting server on port: ", 5844)
	public_api_v1.RunPublicApi_v1(5844)
}
