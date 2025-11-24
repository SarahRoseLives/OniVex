package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"onivex/discovery"

	"github.com/cretz/bine/tor"
)

// UIContext holds data to render in the HTML template
type UIContext struct {
	MyAddress   string
	PeerCount   int
	Peers       []string
	SearchQuery string
	Results     []discovery.SearchResult
}

func Start(port int, myAddress string, pm *discovery.PeerManager, t *tor.Tor) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("🖥️  Starting Web UI at http://%s\n", addr)

	// 1. Main UI Handler (Renders the initial page structure)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// We no longer do the blocking SearchNetwork here.
		// We just render the page with empty results initially.

		peers := pm.GetPeers()
		data := UIContext{
			MyAddress:   myAddress,
			PeerCount:   len(peers),
			Peers:       peers,
			SearchQuery: "",
			Results:     nil,
		}

		tmpl, err := template.ParseGlob("webui/templates/*.html")
		if err != nil {
			http.Error(w, "Could not load templates: "+err.Error(), 500)
			return
		}

		err = tmpl.ExecuteTemplate(w, "layout.html", data)
		if err != nil {
			log.Printf("Template Error: %v", err)
		}
	})

	// 2. AJAX Search Endpoint (NEW)
	// This is called by JavaScript. It blocks, but only the HTTP request blocks,
	// not the user's browser UI.
	http.HandleFunc("/api/ui/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode([]discovery.SearchResult{})
			return
		}

		// Perform the slow network search
		results := pm.SearchNetwork(query)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// 3. Download Proxy Handler
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer")
		filePath := r.URL.Query().Get("path")
		fileName := r.URL.Query().Get("name")

		if peerID == "" || filePath == "" {
			http.Error(w, "Missing peer or path", http.StatusBadRequest)
			return
		}

		localPath := filepath.Join("downloads", fileName)
		outFile, err := os.Create(localPath)
		if err != nil {
			http.Error(w, "Failed to create local file", http.StatusInternalServerError)
			return
		}
		defer outFile.Close()

		var bytesWritten int64

		if peerID == myAddress {
			// Local Copy
			fmt.Printf("📂 Local Download: %s\n", fileName)
			sourcePath := filepath.Join("uploads", filePath)
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				http.Error(w, "Local file not found", 404)
				return
			}
			defer sourceFile.Close()
			bytesWritten, err = io.Copy(outFile, sourceFile)
		} else {
			// Remote Download via Tor
			fmt.Printf("📥 Tor Download: %s from %s\n", fileName, peerID)
			dialer, err := t.Dialer(context.Background(), nil)
			if err != nil {
				http.Error(w, "Tor dialer failed", 500)
				return
			}
			torClient := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				Timeout:   10 * time.Minute,
			}
			targetURL := fmt.Sprintf("http://%s%s", peerID, filePath)
			resp, err := torClient.Get(targetURL)
			if err != nil {
				http.Error(w, "Connection failed", 502)
				return
			}
			defer resp.Body.Close()
			bytesWritten, err = io.Copy(outFile, resp.Body)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "success",
			"path":   localPath,
			"size":   bytesWritten,
		})
	})

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("❌ Web UI failed to start: %v", err)
	}
}