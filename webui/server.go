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

// Start launches the local control panel on localhost
func Start(port int, myAddress string, pm *discovery.PeerManager, t *tor.Tor) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("🖥️  Starting Web UI at http://%s\n", addr)

	// 1. Main UI Handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Check for Search Query
		query := r.URL.Query().Get("q")
		var results []discovery.SearchResult

		if query != "" {
			results = pm.SearchNetwork(query)
		}

		peers := pm.GetPeers()
		data := UIContext{
			MyAddress:   myAddress,
			PeerCount:   len(peers),
			Peers:       peers,
			SearchQuery: query,
			Results:     results,
		}

		// UPDATED: Load all HTML files in the directory
		// This allows layout.html to include header.html, footer.html, etc.
		tmpl, err := template.ParseGlob("webui/templates/*.html")
		if err != nil {
			http.Error(w, "Could not load templates: "+err.Error(), 500)
			return
		}

		// Execute "layout.html" (defined in layout.html) as the entry point
		err = tmpl.ExecuteTemplate(w, "layout.html", data)
		if err != nil {
			log.Printf("Template Error: %v", err)
		}
	})

	// 2. Download Proxy Handler
	// This runs on Localhost. It connects to Tor (or Local Disk) and saves to /downloads.
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer")
		filePath := r.URL.Query().Get("path")
		fileName := r.URL.Query().Get("name")

		if peerID == "" || filePath == "" {
			http.Error(w, "Missing peer or path", http.StatusBadRequest)
			return
		}

		// Create Destination File in 'downloads' folder
		localPath := filepath.Join("downloads", fileName)
		outFile, err := os.Create(localPath)
		if err != nil {
			http.Error(w, "Failed to create local file", http.StatusInternalServerError)
			return
		}
		defer outFile.Close()

		var bytesWritten int64

		// --- LOOPBACK CHECK ---
		// If the file is hosted by ME, copy from disk. Do not use Tor.
		if peerID == myAddress {
			fmt.Printf("📂 Local Download Detected (Bypassing Tor): %s\n", fileName)

			// Construct source path (e.g. uploads/foo.txt)
			sourcePath := filepath.Join("uploads", filePath)
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				http.Error(w, "Could not open local source file", http.StatusNotFound)
				return
			}
			defer sourceFile.Close()

			bytesWritten, err = io.Copy(outFile, sourceFile)
			if err != nil {
				http.Error(w, "Local copy failed", http.StatusInternalServerError)
				return
			}

		} else {
			// --- REMOTE DOWNLOAD ---
			// Use Tor to dial the remote peer
			fmt.Printf("📥 Starting Tor download: %s from %s\n", fileName, peerID)

			dialer, err := t.Dialer(context.Background(), nil)
			if err != nil {
				http.Error(w, "Tor dialer failed", http.StatusInternalServerError)
				return
			}

			torClient := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				Timeout:   10 * time.Minute,
			}

			targetURL := fmt.Sprintf("http://%s%s", peerID, filePath)
			resp, err := torClient.Get(targetURL)
			if err != nil {
				fmt.Printf("❌ Connection Failed: %v\n", err)
				http.Error(w, "Failed to connect to peer: "+err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				http.Error(w, "Peer returned error", resp.StatusCode)
				return
			}

			// Write data from Tor stream to local file
			bytesWritten, err = io.Copy(outFile, resp.Body)
			if err != nil {
				fmt.Printf("❌ Download interrupted: %v\n", err)
				http.Error(w, "Download interrupted", http.StatusInternalServerError)
				return
			}
		}

		fmt.Printf("✅ Download complete: %s (%d bytes)\n", localPath, bytesWritten)

		// Respond with JSON so the UI knows we finished
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