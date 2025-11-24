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
	"onivex/filesystem"

	"github.com/cretz/bine/tor"
)

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

	// 1. Main Page
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, "Template Error: "+err.Error(), 500)
			return
		}
		tmpl.ExecuteTemplate(w, "layout.html", data)
	})

	// 2. AJAX Search Endpoint
	http.HandleFunc("/api/ui/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode([]discovery.SearchResult{})
			return
		}

		// A. Search Local Filesystem (INSTANT)
		localFiles := filesystem.SearchLocal(query)

		// B. Search Network (SLOW) - Pass myAddress to exclude self from Tor search
		networkResults := pm.SearchNetwork(query, myAddress)

		// C. Combine
		finalResults := []discovery.SearchResult{}

		if len(localFiles) > 0 {
			finalResults = append(finalResults, discovery.SearchResult{
				PeerID: myAddress,
				Files:  localFiles,
				Source: "local",
			})
		}
		finalResults = append(finalResults, networkResults...)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(finalResults)
	})

	// 3. Download Proxy
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer")
		filePath := r.URL.Query().Get("path")
		fileName := r.URL.Query().Get("name")

		if peerID == "" || filePath == "" {
			http.Error(w, "Missing params", 400)
			return
		}

		localPath := filepath.Join("downloads", fileName)
		outFile, err := os.Create(localPath)
		if err != nil {
			http.Error(w, "Create file failed", 500)
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
			// Tor Download
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
			resp, err := torClient.Get(fmt.Sprintf("http://%s%s", peerID, filePath))
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