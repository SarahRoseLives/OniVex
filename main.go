package main

import (
	"fmt"
	"log"
	"net/http"

	// Import your local packages
	// Replace "example.com/onivex" with whatever you name your module
	"onivex/filesystem"
	"onivex/network"
)

func main() {
	// 1. Setup File System
	filesystem.EnsureDirectories()

	// 2. Setup Network (Tor)
	onion, err := network.SetupTor()
	if err != nil {
		log.Fatalf("Fatal Network Error: %v", err)
	}
	defer onion.Close()

	fmt.Printf("\n✨ ONIVEX IS LIVE\n")
	fmt.Printf("👉 Onion Address: http://%v.onion\n\n", onion.ID)

	// 3. Setup HTTP Routing
	mux := http.NewServeMux()

	// Route: File Server
	mux.Handle("/", filesystem.GetFileHandler())

	// Route: API Heartbeat (for future peers to ping)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OniVex Online"))
	})

	// 4. Start Server
	// We pass 'onion' (net.Listener) to http.Serve
	log.Fatal(http.Serve(onion, mux))
}