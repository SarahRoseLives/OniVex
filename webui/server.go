package webui

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"onivex/discovery"
)

// UIContext holds data to render in the HTML template
type UIContext struct {
	MyAddress string
	PeerCount int
	Peers     []string
}

// Start launches the local control panel on localhost
func Start(port int, myAddress string, pm *discovery.PeerManager) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("🖥️  Starting Web UI at http://%s\n", addr)

	// Route: Dashboard
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. Gather live data
		peers := pm.GetPeers()
		data := UIContext{
			MyAddress: myAddress,
			PeerCount: len(peers),
			Peers:     peers,
		}

		// 2. Parse Template
		// Note: Path is relative to where you run 'go run main.go'
		tmpl, err := template.ParseFiles("webui/templates/index.html")
		if err != nil {
			http.Error(w, "Could not load template: "+err.Error(), 500)
			return
		}

		// 3. Render
		tmpl.Execute(w, data)
	})

	// Start server in background (blocking, so call via goroutine in main)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("❌ Web UI failed to start: %v", err)
	}
}