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

	t, onion, err := network.SetupTor("seed_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)
	fmt.Printf("\n⭐ SEED ADDRESS (Copy to discovery/bootstrap.go): \n   %s\n\n", myAddress)

	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Seed Node Active"))
	})

	// UPDATED: Handle POST to register new peers
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {

		// 1. If it's a POST, they are announcing themselves
		if r.Method == http.MethodPost {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				addr := payload["addr"]
				if addr != "" {
					fmt.Printf("👋 New Client Announced: %s\n", addr)
					peers.AddPeer(addr)
				}
			}
		}

		// 2. Always return the current list of peers
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	go func() {
		for {
			time.Sleep(1 * time.Hour)
			fmt.Printf("⏱️  Seed Node Heartbeat. Tracking %d peers.\n", len(peers.GetPeers()))
		}
	}()

	log.Fatal(http.Serve(onion, mux))
}