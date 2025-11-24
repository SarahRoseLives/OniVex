package main

import (
	"encoding/json"
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
	// 1. Setup Directories
	filesystem.EnsureDirectories()

	// 2. Setup Tor Network (EPHEMERAL MODE)
	// Passing empty string "" generates a random identity every time.
	t, onion, err := network.SetupTor("")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	// 3. Initialize Discovery
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	// 4. Start Local Web UI
	// Passing 't' allows the UI to proxy downloads through Tor
	go webui.Start(8081, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:8081\n\n")

	// 5. Setup Tor HTTP Routes (Hidden Service)
	mux := http.NewServeMux()

	// Serve files from 'uploads' folder
	mux.Handle("/", filesystem.GetFileHandler())

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	// Allow other peers/seeds to sync with us
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

	// Search Endpoint (Other peers call this)
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results := filesystem.SearchLocal(query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// 6. Background Heartbeat & Bootstrap Loop
	go func() {
		// Initial wait for Tor to stabilize
		fmt.Println("⏳ Waiting for Tor circuit stability (15s)...")
		time.Sleep(15 * time.Second)

		// Loop forever to keep registration alive
		for {
			fmt.Println("💓 Sending Heartbeat to Seeds...")

			// This connects to seeds, downloads their list, AND announces 'myAddress'
			peers.Bootstrap(myAddress)

			// Sleep for 15 minutes (must be less than Seed's 60m timeout)
			time.Sleep(15 * time.Minute)
		}
	}()

	// 7. Block Main Thread
	log.Fatal(http.Serve(onion, mux))
}