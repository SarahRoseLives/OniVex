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
)

func main() {
	// 1. Setup
	filesystem.EnsureDirectories()
	t, onion, err := network.SetupTor()
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)
	fmt.Printf("\n✨ ONIVEX IS LIVE at http://%s\n\n", myAddress)

	// 2. Initialize Discovery
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress) // Add self so we are in the list

	// 3. Setup HTTP Routes
	mux := http.NewServeMux()

	// -> File Server
	mux.Handle("/", filesystem.GetFileHandler())

	// -> API: Status
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	// -> API: Peer Exchange (The Gossip Endpoint)
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		list := peers.GetPeers()
		json.NewEncoder(w).Encode(list)
	})

	// 4. Background Sync Loop
	// In the future, this will loop through ALL known peers.
	// For now, it just talks to itself to prove the concept.
	go func() {
		// Wait for server to start
		time.Sleep(15 * time.Second)

		fmt.Println("\n🔄 Starting Gossip Sync...")
		// We ask "ourselves" for the list.
		// In reality, you would put a friend's onion address here.
		peers.Sync(myAddress)
	}()

	// 5. Start Server
	log.Fatal(http.Serve(onion, mux))
}