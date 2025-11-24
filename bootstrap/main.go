package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"onivex/discovery"
	"onivex/network"
)

func main() {
	fmt.Println("🌳 STARTING ONIVEX SEED NODE 🌳")

	// 1. Setup Tor with a persistent key name "seed_identity"
	// This ensures the Seed always comes back up with the same Onion URL.
	t, onion, err := network.SetupTor("seed_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)
	fmt.Printf("\n⭐ SEED ADDRESS (Copy to discovery/bootstrap.go): \n   %s\n\n", myAddress)

	// 2. Initialize Peer Manager
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	// 3. Start Cleanup Loop (TTL)
	// Check every 10 minutes. If a peer hasn't been seen in 60 minutes, delete them.
	peers.StartCleanup(10*time.Minute, 60*time.Minute)

	// 4. HTTP Routes
	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Seed Node Active"))
	})

	// Peer Discovery API
	// Clients hit this to get the list AND to announce themselves.
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		// If it's a POST, they are announcing themselves
		if r.Method == http.MethodPost {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				addr := payload["addr"]
				if addr != "" && addr != myAddress {
					fmt.Printf("👋 New Client Announced: %s\n", addr)
					peers.AddPeer(addr) // Updates LastSeen timestamp
				}
			}
		}

		// Always return the full list of currently active peers
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	// 5. Local Console Heartbeat
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			fmt.Printf("⏱️  Seed Node Heartbeat. Tracking %d active peers.\n", len(peers.GetPeers()))
		}
	}()

	// 6. Serve
	log.Fatal(http.Serve(onion, mux))
}