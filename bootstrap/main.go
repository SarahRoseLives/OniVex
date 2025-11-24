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
	peers.StartCleanup(10*time.Minute, 60*time.Minute)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Seed Node Active"))
	})

	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				addr := payload["addr"]
				if addr != "" && addr != myAddress {
					fmt.Printf("👋 New Client Announced: %s\n", addr)
					peers.AddPeer(addr)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		// Return larger subset for seeds (200)
		json.NewEncoder(w).Encode(peers.GetRandomPeers(200))
	})

	mux.HandleFunc("/api/index", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	})

	go func() {
		for {
			time.Sleep(1 * time.Hour)
			fmt.Printf("⏱️  Seed Node Heartbeat. Tracking %d active peers.\n", len(peers.GetPeers()))
		}
	}()

	log.Fatal(http.Serve(onion, mux))
}