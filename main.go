package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"onivex/discovery"
	"onivex/filesystem"
	"onivex/network"
	"onivex/webui"
)

func main() {
	// Command line flag for port (default 8080)
	port := flag.Int("port", 8080, "Web UI Port")
	flag.Parse()

	// 1. Setup Directories
	filesystem.EnsureDirectories()

	// 2. Setup Tor Network (PERSISTENT MODE)
	// Changed from "" to "client_identity".
	// This triggers the logic to save/load the key from disk.
	t, onion, err := network.SetupTor("client_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	// 3. Initialize Discovery
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	// Autosave peers.json every 5 minutes
	peers.StartPersistence(5 * time.Minute)

	// 4. Start Local Web UI
	go webui.Start(*port, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:%d\n\n", *port)

	// 5. Setup Tor HTTP Routes
	mux := http.NewServeMux()
	mux.Handle("/", filesystem.GetFileHandler())

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
		json.NewEncoder(w).Encode(peers.GetPeers())
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

	// 6. Heartbeat Loop
	go func() {
		fmt.Println("⏳ Waiting for Tor circuit stability (15s)...")
		time.Sleep(15 * time.Second)
		for {
			// fmt.Println("💓 Heartbeat & Index Sync...")
			peers.Bootstrap(myAddress)
			time.Sleep(15 * time.Minute)
		}
	}()

	// 7. Block Main Thread
	log.Fatal(http.Serve(onion, mux))
}