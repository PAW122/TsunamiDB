package public_api_v1

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	routes "TsunamiDB/servers/public-api/v1/routes"
)

// Konfiguracja connection pool
var transport = &http.Transport{
	MaxIdleConns:        10000,            // Maksymalna liczba połączeń w puli
	MaxIdleConnsPerHost: 10000,            // Maksymalna liczba połączeń na hosta
	IdleConnTimeout:     90 * time.Second, // Limit czasu połączenia w puli
}

// Tworzymy klienta HTTP z connection pool
var client = &http.Client{
	Transport: transport,
	Timeout:   30 * time.Second, // Maksymalny czas requestu
}

func RunPublicApi_v1(port int) {
	mux := http.NewServeMux()
	go mux.HandleFunc("/save/", routes.Save)        // save
	go mux.HandleFunc("/read/", routes.Read)        // read
	go mux.HandleFunc("/free/", routes.Free)        // delete
	go mux.HandleFunc("/stream/", routes.StreamMP4) // stream read

	// Konfiguracja serwera z Connection Pool
	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,  // Limit czasu na odczyt requestu
		WriteTimeout:   10 * time.Second,  // Limit czasu na odpowiedź
		IdleTimeout:    120 * time.Second, // Limit czasu na utrzymywanie połączenia
		MaxHeaderBytes: 1 << 20,           // 1MB nagłówków
	}

	// Ustawienie limitu jednoczesnych połączeń (Linux, Windows)
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Public API running on port", port)
	err = server.Serve(listener)
	if err != nil {
		log.Fatal(err)
	}
}
