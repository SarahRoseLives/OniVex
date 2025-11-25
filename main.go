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
	"onivex/discovery"
	"onivex/filesystem"
	"onivex/network"
	"onivex/webui"
)

// Wrapper to log file access requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only log actual file requests (ignore /api calls which are logged elsewhere)
		if len(r.URL.Path) > 1 && r.URL.Path[0:4] != "/api" {
			fmt.Printf("📂 Serving File: %s to %s\n", r.URL.Path, r.RemoteAddr)
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

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:%d\n\n", *port)

	mux := http.NewServeMux()

	// Wrap the file handler with logging
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

	// --- UPDATED FILTER HANDLER (Tokenization Fix) ---
	mux.HandleFunc("/api/filter", func(w http.ResponseWriter, r *http.Request) {
		files, _ := filesystem.GetFileList()
		filter := bloom.New(1000, 0.01)
		for _, f := range files {
			name := strings.ToLower(f.Name)

			// 1. Add the exact full filename (e.g., "test.md")
			filter.Add([]byte(name))

			// 2. Tokenize: Split by common delimiters to allow keyword searching
			// This splits "my_cool_file.txt" into ["my", "cool", "file", "txt"]
			tokens := strings.FieldsFunc(name, func(r rune) bool {
				return r == '.' || r == ' ' || r == '_' || r == '-'
			})

			for _, token := range tokens {
				// Add the keyword to the filter if it's not empty
				if len(token) > 0 {
					filter.Add([]byte(token))
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(filter)
	})
	// -------------------------------------------------

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

	log.Fatal(http.Serve(onion, mux))
}