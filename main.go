package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"onivex/bloom"
	"onivex/config" // <--- IMPORTED
	"onivex/discovery"
	"onivex/filesystem"
	"onivex/network"
	"onivex/webui"
)

// Wrapper to log file access requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 1 && r.URL.Path[0:4] != "/api" {
			fmt.Printf("📂 Serving File: %s to %s\n", r.URL.Path, r.RemoteAddr)
		}
		next.ServeHTTP(w, r)
	})
}

// Validates incoming versions and stamps outgoing responses
func versionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Tag response with our version
		w.Header().Set("X-Onivex-Version", config.ProtocolVersion) // <--- UPDATED

		// 2. Check client version (Logging only for now)
		clientVer := r.Header.Get("X-Onivex-Version")
		if clientVer != "" && clientVer != config.ProtocolVersion { // <--- UPDATED
			// fmt.Printf("⚠️  Peer Version Mismatch: %s (Them) vs %s (Us)\n", clientVer, config.ProtocolVersion)
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.Int("port", 8080, "Web UI Port")
	flag.Parse()

	filesystem.EnsureDirectories()

	t, onion, err := network.SetupTor("client_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)
	peers.StartPersistence(5 * time.Minute)

	go webui.Start(*port, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE (v%s)\n", config.ProtocolVersion) // <--- UPDATED
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:%d\n\n", *port)

	mux := http.NewServeMux()

	fileHandler := filesystem.GetFileHandler()
	mux.Handle("/", loggingMiddleware(fileHandler))

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				if addr := payload["addr"]; addr != "" && addr != myAddress {
					peers.AddPeer(addr)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetRandomPeers(50))
	})

	mux.HandleFunc("/api/filter", func(w http.ResponseWriter, r *http.Request) {
		files, _ := filesystem.GetFileList()
		filter := bloom.New(1000, 0.01)
		for _, f := range files {
			name := strings.ToLower(f.Name)
			filter.Add([]byte(name))
			tokens := strings.FieldsFunc(name, func(r rune) bool {
				return r == '.' || r == ' ' || r == '_' || r == '-'
			})
			for _, token := range tokens {
				if len(token) > 0 {
					filter.Add([]byte(token))
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filter)
	})

	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results := filesystem.SearchLocal(query)
		if len(results) > 0 {
			fmt.Printf("💡 Found match for forwarded query '%s'\n", query)
		}
	})

	mux.HandleFunc("/api/index", func(w http.ResponseWriter, r *http.Request) {
		files, _ := filesystem.GetFileList()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results := filesystem.SearchLocal(query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	go func() {
		fmt.Println("⏳ Waiting for Tor circuit stability (15s)...")
		time.Sleep(15 * time.Second)
		for {
			peers.Bootstrap(myAddress)
			time.Sleep(15 * time.Minute)
		}
	}()

	log.Fatal(http.Serve(onion, versionMiddleware(mux)))
}