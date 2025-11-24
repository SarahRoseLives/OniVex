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
	port := flag.Int("port", 8080, "Web UI Port")
	flag.Parse()

	filesystem.EnsureDirectories()

	t, onion, err := network.SetupTor("client_identity")
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer t.Close()
	defer onion.Close()

	myAddress := fmt.Sprintf("%v.onion", onion.ID)

	peers := discovery.NewPeerManager(t)
	peers.AddPeer(myAddress)
	peers.StartPersistence(5 * time.Minute)

	go webui.Start(*port, myAddress, peers, t)

	fmt.Printf("\n✨ ONIVEX CLIENT LIVE\n")
	fmt.Printf("👉 Tor Access: http://%s\n", myAddress)
	fmt.Printf("👉 Control UI: http://127.0.0.1:%d\n\n", *port)

	mux := http.NewServeMux()
	mux.Handle("/", filesystem.GetFileHandler())

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	// UPDATED: Return Random Subset
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

		// Return 50 random peers to facilitate gossip without huge payloads
		json.NewEncoder(w).Encode(peers.GetRandomPeers(50))
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

	go func() {
		fmt.Println("⏳ Waiting for Tor circuit stability (15s)...")
		time.Sleep(15 * time.Second)
		for {
			// fmt.Println("💓 Heartbeat & Gossip...")
			peers.Bootstrap(myAddress)
			time.Sleep(15 * time.Minute)
		}
	}()

	log.Fatal(http.Serve(onion, mux))
}