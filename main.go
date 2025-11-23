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

	// 2. Setup Tor Network
	t, onion, err := network.SetupTor()
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("http://%v.onion", onion.ID)

	// 3. Initialize Discovery
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(onion.ID + ".onion") // Add self for testing

	// 4. Start Local Web UI (Non-blocking goroutine)
	// This runs on localhost:8080 so you can control the app
	go webui.Start(8080, myAddress, peers)

	fmt.Printf("\n✨ ONIVEX IS LIVE\n")
	fmt.Printf("👉 Tor Access: %s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:8080\n\n")

	// 5. Setup Tor HTTP Routes (The "Hidden Service" Logic)
	mux := http.NewServeMux()
	mux.Handle("/", filesystem.GetFileHandler())

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	// 6. Background Gossip Loop (Test)
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("\n🔄 Starting Gossip Sync...")
		peers.Sync(onion.ID + ".onion")
	}()

	// 7. Block Main Thread with Tor Server
	log.Fatal(http.Serve(onion, mux))
}