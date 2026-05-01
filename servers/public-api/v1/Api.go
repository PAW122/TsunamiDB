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

var transport = &http.Transport{
	MaxIdleConns:        10000,
	MaxIdleConnsPerHost: 10000,
	MaxConnsPerHost:     10000,
	IdleConnTimeout:     90 * time.Second,
	DisableKeepAlives:   false,
	ForceAttemptHTTP2:   true,
}

var HTTPClient = &http.Client{
	Transport: transport,
	Timeout:   30 * time.Second,
}

func withClient(fn func(http.ResponseWriter, *http.Request, *http.Client)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			metrics.RecordRequest(time.Since(start))
		}()
		fn(w, r, HTTPClient)
	}
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()

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
	mux.HandleFunc("/sql", withClient(routes.SQL_api))
	mux.HandleFunc("/key_by_regex/", withClient(routes.GetKeysByRegex))
	mux.HandleFunc("/health", withClient(routes.Health))

	return mux
}

func newServer(port int, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

func servePublicAPI(port int) error {
	server := newServer(port, newMux())

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("cannot start listener: %w", err)
	}

	fmt.Printf("Public API v1 listening on :%d (keep-alive, HTTP/2 when TLS is enabled)\n", port)

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func RunPublicApi_v1(port int) {
	if err := servePublicAPI(port); err != nil {
		log.Fatal(err)
	}
}
