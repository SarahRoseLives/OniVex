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
	filesystem.EnsureDirectories()

	// Client Mode (Ephemeral Key)
	t, onion, err := network.SetupTor("")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)

	go webui.Start(8080, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:8080\n\n")

	mux := http.NewServeMux()
	mux.Handle("/", filesystem.GetFileHandler())

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	// UPDATED: Allow peers to announce themselves to us as well
	mux.HandleFunc("/api/peers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				if addr := payload["addr"]; addr != "" {
					peers.AddPeer(addr)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers.GetPeers())
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		results := filesystem.SearchLocal(query)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// Start Bootstrap
	go func() {
		time.Sleep(15 * time.Second)
		// UPDATED: Pass myAddress so we can register with the seed
		peers.Bootstrap(myAddress)
	}()

	log.Fatal(http.Serve(onion, mux))
}