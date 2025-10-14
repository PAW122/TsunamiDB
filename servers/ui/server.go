package ui

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
	routes "github.com/PAW122/TsunamiDB/servers/public-api/v1/routes"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

//go:embed assets/*
var embeddedAssets embed.FS

// RunAdminUI starts the optional admin UI on the provided port.
func RunAdminUI(port int) error {
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return fmt.Errorf("prepare assets: %w", err)
	}

	indexHTML, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/", "/index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexHTML)
		default:
			http.NotFound(w, r)
		}
	})

	registerAPI(mux)
	registerAdminRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      withSecurityHeaders(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Admin UI listening on :%d", port)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func registerAPI(mux *http.ServeMux) {
	adapter := func(fn func(http.ResponseWriter, *http.Request, *http.Client)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			fn(w, r, public_api_v1.HTTPClient)
		}
	}

	mux.HandleFunc("/api/save/", adapter(routes.AsyncSave))
	mux.HandleFunc("/api/read/", adapter(routes.AsyncRead))
	mux.HandleFunc("/api/free/", adapter(routes.Free))
	mux.HandleFunc("/api/save_encrypted/", adapter(routes.SaveEncrypted))
	mux.HandleFunc("/api/read_encrypted/", adapter(routes.ReadEncrypted))
	mux.HandleFunc("/api/save_inc/", adapter(routes.SaveIncremental))
	mux.HandleFunc("/api/read_inc/", adapter(routes.ReadIncremental))
	mux.HandleFunc("/api/delete_inc/", adapter(routes.DeleteIncremental))
	mux.HandleFunc("/api/sql", adapter(routes.SQL_api))
	mux.HandleFunc("/api/key_by_regex/", adapter(routes.GetKeysByRegex))
	mux.HandleFunc("/api/health", adapter(routes.Health))
	mux.HandleFunc("/api/subscriptions/enable", adapter(subServer.HandleEnableSubscription))
	mux.HandleFunc("/api/subscriptions/disable", adapter(subServer.HandleDisableSubscription))
	mux.HandleFunc("/api/subscriptions/stats", handleSubscriptionStats)
}

func registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/tables", handleTablesList)
	mux.HandleFunc("/api/admin/inc_descriptors", handleIncDescriptors)

	mux.HandleFunc("/api/admin/tables/", func(w http.ResponseWriter, r *http.Request) {
		segments, err := parseSegments(r.URL.Path, "/api/admin/tables/")
		if err != nil || len(segments) < 2 {
			http.NotFound(w, r)
			return
		}
		table := segments[0]
		if segments[1] != "entries" {
			http.NotFound(w, r)
			return
		}

		if len(segments) == 2 {
			handleTableEntries(w, r, table)
			return
		}
		if len(segments) == 3 {
			handleEntryDetail(w, r, table, segments[2])
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("/api/admin/inc_tables/", func(w http.ResponseWriter, r *http.Request) {
		segments, err := parseSegments(r.URL.Path, "/api/admin/inc_tables/")
		if err != nil || len(segments) != 2 {
			http.NotFound(w, r)
			return
		}
		handleIncEntries(w, r, segments[0], segments[1])
	})
}

func handleSubscriptionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stats := subServer.StatsSnapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(stats)
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
