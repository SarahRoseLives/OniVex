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
	"strings"
	"time"

	"onivex/config" // <--- IMPORTED
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

	http.Handle("/library/files/", http.StripPrefix("/library/files/", http.FileServer(http.Dir("downloads"))))

	http.HandleFunc("/api/library", func(w http.ResponseWriter, r *http.Request) {
		files, err := filesystem.GetDownloadsList()
		if err != nil {
			http.Error(w, "Failed to scan library", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(files)
	})

	http.HandleFunc("/api/ui/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			json.NewEncoder(w).Encode([]discovery.SearchResult{})
			return
		}

		localFiles := filesystem.SearchLocal(query)
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

	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		peerID := r.URL.Query().Get("peer")
		filePath := r.URL.Query().Get("path")
		fileName := r.URL.Query().Get("name")

		if peerID == "" || filePath == "" {
			http.Error(w, "Missing params", 400)
			return
		}

		cleanPath := strings.TrimLeft(filePath, "/\\")
		cleanPath = filepath.Clean(cleanPath)

		if strings.Contains(cleanPath, "..") || filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, "/") || strings.HasPrefix(cleanPath, "\\") {
			fmt.Printf("🚨 Security Warning: Blocked path traversal attempt: %s\n", filePath)
			http.Error(w, "Security Violation: Invalid File Path", 403)
			return
		}

		os.MkdirAll("downloads", 0755)
		localFileName := filepath.Base(fileName)
		localPath := filepath.Join("downloads", localFileName)

		outFile, err := os.Create(localPath)
		if err != nil {
			http.Error(w, "Create file failed", 500)
			return
		}
		defer outFile.Close()

		var bytesWritten int64

		if peerID == myAddress {
			fmt.Printf("📂 Local Download: %s\n", localFileName)
			sourcePath := filepath.Join("uploads", cleanPath)
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				http.Error(w, "Local file not found", 404)
				return
			}
			defer sourceFile.Close()
			bytesWritten, err = io.Copy(outFile, sourceFile)
		} else {
			fmt.Printf("📥 Tor Download: %s from %s\n", localFileName, peerID)

			dialCtx, dialCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer dialCancel()

			dialer, err := t.Dialer(dialCtx, nil)
			if err != nil {
				fmt.Printf("   ❌ Dialer Error: %v\n", err)
				http.Error(w, "Tor dialer failed", 500)
				return
			}

			torClient := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				Timeout:   15 * time.Minute,
			}

			targetURL := fmt.Sprintf("http://%s/%s", peerID, cleanPath)

			// VERSIONED REQUEST
			req, err := http.NewRequest("GET", targetURL, nil)
			if err != nil {
				http.Error(w, "Request creation failed", 500)
				return
			}
			req.Header.Set("X-Onivex-Version", config.ProtocolVersion) // <--- UPDATED

			resp, err := torClient.Do(req)
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