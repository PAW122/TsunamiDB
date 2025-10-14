package core

import (
	"fmt"
	"strconv"
	"strings"
)

type runOptions struct {
	corePort   int
	uiPort     int
	loadConfig bool
	peers      []string
}

const usageMessage = "Uzycie: TsunamiDB <port> [peer1] [peer2] ... [-config] [-ui <port>]"

func parseRunOptions(args []string) (runOptions, error) {
	var (
		opts    runOptions
		portStr string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 0 && arg[0] == '-' {
			trimmed := strings.TrimLeft(arg, "-")
			if trimmed == "" {
				return opts, fmt.Errorf("nieznana flaga: %s", arg)
			}

			name := trimmed
			value := ""
			if strings.Contains(trimmed, "=") {
				var ok bool
				name, value, ok = strings.Cut(trimmed, "=")
				if !ok {
					name = trimmed
				}
			}

			switch name {
			case "config":
				if value != "" {
					return opts, fmt.Errorf("flaga -config nie przyjmuje wartosci")
				}
				opts.loadConfig = true
			case "ui":
				if value == "" {
					if i+1 >= len(args) {
						return opts, fmt.Errorf("brak portu po fladze -ui")
					}
					value = args[i+1]
					i++
				}
				port, err := strconv.Atoi(value)
				if err != nil {
					return opts, fmt.Errorf("niepoprawny port UI: %v", err)
				}
				if err := validatePort(port); err != nil {
					return opts, fmt.Errorf("niepoprawny port UI: %w", err)
				}
				opts.uiPort = port
			default:
				return opts, fmt.Errorf("nieznana flaga: %s", arg)
			}
			continue
		}

		if portStr == "" {
			portStr = arg
			continue
		}
		opts.peers = append(opts.peers, arg)
	}

	if portStr == "" {
		return opts, fmt.Errorf("brak portu serwera")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return opts, fmt.Errorf("niepoprawny port serwera: %v", err)
	}
	if err := validatePort(port); err != nil {
		return opts, fmt.Errorf("niepoprawny port serwera: %w", err)
	}
	opts.corePort = port

	return opts, nil
}

func validatePort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port musi byc w zakresie 1-65535")
	}
	return nil
}
