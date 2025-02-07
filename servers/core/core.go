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

	public_api_v1 "TsunamiDB/servers/public-api/v1"
)

func RunCore() {
	config := flag.Bool("config", false, "load config from config.json")
	flag.Parse()

	if *config {
		fmt.Println("load config")
	}

	fmt.Println("Starting server on port: ", 5844)
	public_api_v1.RunPublicApi_v1(5844)
}
