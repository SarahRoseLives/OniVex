package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"onivex/filesystem"

	"github.com/cretz/bine/tor"
)

// PeerManager handles the list of known neighbors
type PeerManager struct {
	mu         sync.RWMutex
	KnownPeers map[string]bool
	Tor        *tor.Tor
}

// SearchResult holds data returned from a remote peer
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

	if onionAddr == "" {
		return
	}

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

// Bootstrap connects to the hardcoded seed nodes
// UPDATED: Requires myOnionAddr so we can tell the seed who we are
func (pm *PeerManager) Bootstrap(myOnionAddr string) {
	if len(BootstrapPeers) == 0 {
		fmt.Println("⚠️ No bootstrap peers configured.")
		return
	}

	fmt.Println("🌍 Bootstrapping network connection...")
	for _, seed := range BootstrapPeers {
		// Don't sync with ourselves if we are the seed
		if seed != myOnionAddr {
			go pm.Sync(seed, myOnionAddr)
		}
	}
}

// Sync connects to a target and exchanges peer lists
// UPDATED: Sends a POST request with 'myAddr' to announce existence
func (pm *PeerManager) Sync(targetPeer string, myAddr string) {
	fmt.Printf("📡 Syncing with %s...\n", targetPeer)

	dialer, err := pm.Tor.Dialer(context.Background(), nil)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}

	// 1. Prepare Payload (Me)
	payload := map[string]string{"addr": myAddr}
	jsonData, _ := json.Marshal(payload)

	// 2. POST to the peer (Announce myself + Get their list)
	resp, err := httpClient.Post(
		"http://"+targetPeer+"/api/peers",
		"application/json",
		bytes.NewBuffer(jsonData),
	)

	if err != nil {
		fmt.Printf("❌ Sync Failed with %s: %v\n", targetPeer, err)
		return
	}
	defer resp.Body.Close()

	// 3. Decode their list of peers
	var newPeers []string
	if err := json.NewDecoder(resp.Body).Decode(&newPeers); err != nil {
		return
	}

	count := 0
	for _, p := range newPeers {
		// Don't add ourselves if the seed sends it back
		if p != myAddr {
			pm.AddPeer(p)
			count++
		}
	}
	fmt.Printf("✅ Sync Complete. Learned %d peers from %s\n", count, targetPeer)
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