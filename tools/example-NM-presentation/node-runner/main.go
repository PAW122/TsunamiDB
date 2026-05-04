package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

func main() {
	var apiPort int
	var subPort int
	var nmPort int
	var advertiseAddr string
	var knownPeersCSV string
	var networkSecret string

	flag.IntVar(&apiPort, "api-port", 5844, "HTTP API port")
	flag.IntVar(&subPort, "sub-port", 5845, "subscription WebSocket port")
	flag.IntVar(&nmPort, "nm-port", 0, "network-manager WebSocket port")
	flag.StringVar(&advertiseAddr, "advertise-addr", "", "advertise address for network-manager")
	flag.StringVar(&knownPeersCSV, "known-peers", "", "comma-separated peer list")
	flag.StringVar(&networkSecret, "network-secret", "", "shared secret for network-manager transport")
	flag.Parse()

	if advertiseAddr != "" {
		if err := os.Setenv("TSUNAMI_NETWORK_ADVERTISE_ADDR", advertiseAddr); err != nil {
			log.Fatal(err)
		}
	}
	if networkSecret != "" {
		if err := os.Setenv("TSUNAMI_NETWORK_SECRET", networkSecret); err != nil {
			log.Fatal(err)
		}
	}

	knownPeers := splitCSV(knownPeersCSV)
	if nmPort > 0 {
		go networkmanager.StartNetworkManager(nmPort, knownPeers)
	}

	go func() {
		if err := subServer.StartWSServer(strconv.Itoa(subPort)); err != nil {
			log.Fatal(err)
		}
	}()

	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf(
		"node-runner cwd=%s api=%d sub=%d nm=%d advertise=%q peers=%v",
		wd, apiPort, subPort, nmPort, advertiseAddr, knownPeers,
	)

	public_api_v1.RunPublicApi_v1(apiPort)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
