package core

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	configpkg "github.com/PAW122/TsunamiDB/servers/config"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	mysqlcompat "github.com/PAW122/TsunamiDB/servers/mysqlcompat"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

var defaultConfigDir = "./config.json"

type coreDeps struct {
	args                []string
	loadConfig          func(string)
	startNetworkManager func(int, []string)
	startWSServer       func(string) error
	startMySQLServer    func(int) error
	runPublicAPI        func(int)
}

func defaultCoreDeps(args []string) coreDeps {
	return coreDeps{
		args:                args,
		loadConfig:          configpkg.LoadConfig,
		startNetworkManager: networkmanager.StartNetworkManager,
		startWSServer:       subServer.StartWSServer,
		startMySQLServer:    mysqlcompat.Run,
		runPublicAPI:        public_api_v1.RunPublicApi_v1,
	}
}

func RunCore() {
	if err := runCore(defaultCoreDeps(os.Args[1:])); err != nil {
		log.Fatal(err)
	}
}

func runCore(deps coreDeps) error {
	debug.Log("Load config")
	deps.loadConfig(defaultConfigDir)

	debug.Log("Run Core")
	fs := flag.NewFlagSet("core", flag.ContinueOnError)
	loadConfig := fs.Bool("config", false, "load config from config.json")
	if err := fs.Parse(deps.args); err != nil {
		return err
	}

	if *loadConfig {
		fmt.Println("load config")
	}

	args := fs.Args()
	if len(args) > 0 {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid port: %w", err)
		}
		fmt.Println("Starting network manager on port: ", port)

		var knownPeers []string
		if len(args) > 1 {
			knownPeers = args[1:]
		}

		deps.startNetworkManager(port, knownPeers)
	} else {
		fmt.Println("Network manager disabled: no port provided")
	}

	fmt.Println("Starting sub Sever on port:", 5845)
	go func() {
		if err := deps.startWSServer("5845"); err != nil {
			log.Println("subscription server stopped:", err)
		}
	}()

	mysqlPort := mysqlCompatPort()
	fmt.Println("Starting MySQL compatibility server on port:", mysqlPort)
	go func() {
		if err := deps.startMySQLServer(mysqlPort); err != nil {
			log.Println("mysql compatibility server stopped:", err)
		}
	}()

	fmt.Println("Starting server on port: ", 5844)
	deps.runPublicAPI(5844)
	return nil
}

func mysqlCompatPort() int {
	raw := strings.TrimSpace(os.Getenv("TSU_MYSQL_PORT"))
	if raw == "" {
		return 3307
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return 3307
	}
	return port
}
