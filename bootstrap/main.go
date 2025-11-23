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
	t, onion, err := network.SetupTor("seed_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)
	fmt.Printf("\n⭐ SEED ADDRESS (Copy to discovery/bootstrap.go): \n   %s\n\n", myAddress)

	// 2. Initialize Peer Manager (Self-aware)
	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	// 3. HTTP Routes for the Seed
	mux := http.NewServeMux()

	// Health Check
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Seed Node Active"))
	})

	// Peer Discovery API
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// In a production seed, we would capture the requester's IP/Onion here
		// and add them to our list. For now, we serve what we know.
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	// 4. Heartbeat Loop
	go func() {
		for {
			time.Sleep(1 * time.Hour)
			fmt.Printf("⏱️  Seed Node Heartbeat. Tracking %d peers.\n", len(peers.GetPeers()))
		}
	}()

	// 5. Serve
	log.Fatal(http.Serve(onion, mux))
}