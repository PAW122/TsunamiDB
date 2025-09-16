package public_api_v1

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	metrics "github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
	routes "github.com/PAW122/TsunamiDB/servers/public-api/v1/routes"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

// ----------  PULA POŁĄCZEŃ DLA WYCHODZĄCYCH REQUESTÓW  ----------

// Konfiguracja transportu (wspólna pula)
var transport = &http.Transport{
	MaxIdleConns:        10000,
	MaxIdleConnsPerHost: 10000,
	MaxConnsPerHost:     10000,
	IdleConnTimeout:     90 * time.Second,
	DisableKeepAlives:   false, // wymuś keep-alive
	ForceAttemptHTTP2:   true,  // HTTP/2 = multiplexing
}

// Eksportujemy, gdyby ktoś chciał użyć bez wrappera
var HTTPClient = &http.Client{
	Transport: transport,
	Timeout:   30 * time.Second,
}

// ----------  HANDLERY Z WSTRZYKNIĘTYM KLIENTEM  ----------

// Adapter: zamienia handler przyjmujący (*http.Client) na http.HandlerFunc
func withClient(fn func(http.ResponseWriter, *http.Request, *http.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			metrics.RecordRequest(time.Since(start))
		}()
		fn(w, r, HTTPClient)
	}
}

func RunPublicApi_v1(port int) {
	mux := http.NewServeMux()

	// —— zapisy / odczyty ——
	mux.HandleFunc("/save/", withClient(routes.AsyncSave))
	mux.HandleFunc("/read/", withClient(routes.AsyncRead))
	mux.HandleFunc("/free/", withClient(routes.Free))
	mux.HandleFunc("/save_encrypted/", withClient(routes.SaveEncrypted))
	mux.HandleFunc("/read_encrypted/", withClient(routes.ReadEncrypted))
	mux.HandleFunc("/subscriptions/enable", withClient(subServer.HandleEnableSubscription))
	mux.HandleFunc("/subscriptions/disable", withClient(subServer.HandleDisableSubscription))
	mux.HandleFunc("/save_inc/", withClient(routes.SaveIncremental))
	mux.HandleFunc("/read_inc/", withClient(routes.ReadIncremental))
	mux.HandleFunc("/delete_inc/", withClient(routes.DeleteIncremental))

	// —— operacje meta ——
	mux.HandleFunc("/sql", withClient(routes.SQL_api))
	mux.HandleFunc("/key_by_regex/", withClient(routes.GetKeysByRegex))
	mux.HandleFunc("/health", withClient(routes.Health))

	// ------- serwer HTTP --------
	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalf("Nie można uruchomić listenera: %v", err)
	}

	fmt.Printf("Public API v1 nasłuchuje na :%d (keep-alive, HTTP/2 włączone jeśli TLS)\n", port)

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Błąd serwera: %v", err)
	}
}
