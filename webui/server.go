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
	MyAddress   string
	PeerCount   int
	Peers       []string
	SearchQuery string
	Results     []discovery.SearchResult
}

// Start launches the local control panel on localhost
func Start(port int, myAddress string, pm *discovery.PeerManager) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("🖥️  Starting Web UI at http://%s\n", addr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. Check for Search Query
		query := r.URL.Query().Get("q")
		var results []discovery.SearchResult

		if query != "" {
			// Perform the network search if query exists
			results = pm.SearchNetwork(query)
		}

		// 2. Prepare Data
		peers := pm.GetPeers()
		data := UIContext{
			MyAddress:   myAddress,
			PeerCount:   len(peers),
			Peers:       peers,
			SearchQuery: query,
			Results:     results,
		}

		// 3. Render Template
		tmpl, err := template.ParseFiles("webui/templates/index.html")
		if err != nil {
			http.Error(w, "Could not load template: "+err.Error(), 500)
			return
		}

		tmpl.Execute(w, data)
	})

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("❌ Web UI failed to start: %v", err)
	}
}