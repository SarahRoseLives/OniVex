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
	// Passing an empty string "" means we generate a random identity every time.
	// Clients don't need to be persistent, only seeds do.
	t, onion, err := network.SetupTor("")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	// 3. Initialize Discovery
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress) // Add self for testing

	// 4. Start Local Web UI
	// Pass 't' so the WebUI can dial Tor for downloads
	go webui.Start(8080, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:8080\n\n")

	// 5. Setup Tor HTTP Routes
	mux := http.NewServeMux()
	mux.Handle("/", filesystem.GetFileHandler())

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results := filesystem.SearchLocal(query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// 6. Background Bootstrap & Gossip
	go func() {
		// Give Tor a moment to build circuits before trying to bootstrap
		time.Sleep(15 * time.Second)

		// Connect to the Seed Nodes defined in discovery/bootstrap.go
		peers.Bootstrap()
	}()

	// 7. Block Main Thread with Tor Server
	log.Fatal(http.Serve(onion, mux))
}