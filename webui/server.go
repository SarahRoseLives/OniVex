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

	http.HandleFunc("/api/ui/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode([]discovery.SearchResult{})
			return
		}

		// A. Local
		localFiles := filesystem.SearchLocal(query)

		// B. Network (With logging in console)
		networkResults := pm.SearchNetwork(query, myAddress)

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

	// 3. Download Handler (Robust)
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
			// Local
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
			// Network (With Timeouts)
			fmt.Printf("📥 Tor Download: %s from %s\n", fileName, peerID)

			// FIX: Add Timeout to Dialer Context
			dialCtx, dialCancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer dialCancel()

			dialer, err := t.Dialer(dialCtx, nil)
			if err != nil {
				fmt.Printf("   ❌ Dialer Error: %v\n", err)
				http.Error(w, "Tor dialer failed", 500)
				return
			}

			torClient := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				Timeout:   15 * time.Minute, // Long timeout for transfer
			}

			resp, err := torClient.Get(fmt.Sprintf("http://%s%s", peerID, filePath))
			if err != nil {
				fmt.Printf("   ❌ Request Failed: %v\n", err)
				http.Error(w, "Connection failed", 502)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				fmt.Printf("   ❌ Peer Error: %d\n", resp.StatusCode)
				http.Error(w, "Peer returned error", resp.StatusCode)
				return
			}

			fmt.Printf("   ✅ Connected! Downloading...\n")
			bytesWritten, err = io.Copy(outFile, resp.Body)
			if err != nil {
				fmt.Printf("   ❌ Stream Error: %v\n", err)
			}
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