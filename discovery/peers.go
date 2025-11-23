package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"onivex/filesystem" // Now importing filesystem for shared structs

	"github.com/cretz/bine/tor"
)

// PeerManager handles the list of known neighbors
type PeerManager struct {
	mu         sync.RWMutex
	KnownPeers map[string]bool
	Tor        *tor.Tor
}

// SearchResult holds data returned from a remote peer
// UPDATED: Now uses strict filesystem.FileMeta instead of raw JSON
type SearchResult struct {
	PeerID string                `json:"peer_id"`
	Files  []filesystem.FileMeta `json:"files"`
}

func NewPeerManager(t *tor.Tor) *PeerManager {
	return &PeerManager{
		KnownPeers: make(map[string]bool),
		Tor:        t,
	}
}

func (pm *PeerManager) AddPeer(onionAddr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.KnownPeers[onionAddr] {
		pm.KnownPeers[onionAddr] = true
		fmt.Printf("🔭 New Peer Discovered: %s\n", onionAddr)
	}
}

func (pm *PeerManager) GetPeers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	list := []string{}
	for p := range pm.KnownPeers {
		list = append(list, p)
	}
	return list
}

func (pm *PeerManager) Sync(onionAddr string) {
	fmt.Printf("📡 Syncing with %s...\n", onionAddr)

	dialer, err := pm.Tor.Dialer(context.Background(), nil)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}

	resp, err := httpClient.Get("http://" + onionAddr + "/api/peers")
	if err != nil {
		fmt.Printf("❌ Sync Failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var newPeers []string
	if err := json.NewDecoder(resp.Body).Decode(&newPeers); err != nil {
		return
	}

	count := 0
	for _, p := range newPeers {
		pm.AddPeer(p)
		count++
	}
	fmt.Printf("✅ Sync Complete. Learned %d peers from %s\n", count, onionAddr)
}

func (pm *PeerManager) SearchNetwork(query string) []SearchResult {
	peers := pm.GetPeers()
	fmt.Printf("🔍 Searching %d peers for '%s'...\n", len(peers), query)

	results := []SearchResult{}

	for _, p := range peers {
		dialer, err := pm.Tor.Dialer(context.Background(), nil)
		if err != nil {
			continue
		}
		client := &http.Client{
			Transport: &http.Transport{DialContext: dialer.DialContext},
			Timeout:   15 * time.Second,
		}

		url := fmt.Sprintf("http://%s/api/search?q=%s", p, query)
		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("⚠️ Peer %s failed: %v\n", p, err)
			continue
		}

		// UPDATED: Decode directly into FileMeta slice
		var remoteFiles []filesystem.FileMeta
		if err := json.NewDecoder(resp.Body).Decode(&remoteFiles); err == nil {
			if len(remoteFiles) > 0 {
				results = append(results, SearchResult{
					PeerID: p,
					Files:  remoteFiles,
				})
			}
		}
		resp.Body.Close()
	}

	return results
}