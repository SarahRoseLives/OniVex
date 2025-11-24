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
	// UPDATED: Map now stores Last Seen Timestamp
	KnownPeers map[string]time.Time
	Tor        *tor.Tor
}

type SearchResult struct {
	PeerID string                `json:"peer_id"`
	Files  []filesystem.FileMeta `json:"files"`
}

func NewPeerManager(t *tor.Tor) *PeerManager {
	return &PeerManager{
		KnownPeers: make(map[string]time.Time),
		Tor:        t,
	}
}

// AddPeer updates the last-seen timestamp for a peer
func (pm *PeerManager) AddPeer(onionAddr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if onionAddr == "" {
		return
	}

	// If new, log it
	if _, exists := pm.KnownPeers[onionAddr]; !exists {
		fmt.Printf("🔭 New Peer Discovered: %s\n", onionAddr)
	}

	// Update timestamp to NOW
	pm.KnownPeers[onionAddr] = time.Now()
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

// StartCleanup runs a background loop to remove dead peers
// Call this in your main.go
func (pm *PeerManager) StartCleanup(interval time.Duration, peerTimeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			pm.mu.Lock()
			now := time.Now()
			count := 0
			for peer, lastSeen := range pm.KnownPeers {
				if now.Sub(lastSeen) > peerTimeout {
					delete(pm.KnownPeers, peer)
					count++
					fmt.Printf("💀 Pruning dead peer: %s\n", peer)
				}
			}
			pm.mu.Unlock()

			if count > 0 {
				fmt.Printf("🧹 Cleanup: Removed %d inactive peers\n", count)
			}
		}
	}()
}

func (pm *PeerManager) Bootstrap(myOnionAddr string) {
	if len(BootstrapPeers) == 0 {
		fmt.Println("⚠️ No bootstrap peers configured.")
		return
	}

	fmt.Println("🌍 Bootstrapping network connection...")
	for _, seed := range BootstrapPeers {
		if seed != myOnionAddr {
			go pm.Sync(seed, myOnionAddr)
		}
	}
}

func (pm *PeerManager) Sync(targetPeer string, myAddr string) {
	// (Sync code remains exactly the same as previous step)
	// ...
	fmt.Printf("📡 Syncing with %s...\n", targetPeer)

	dialer, err := pm.Tor.Dialer(context.Background(), nil)
	if err != nil {
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}

	payload := map[string]string{"addr": myAddr}
	jsonData, _ := json.Marshal(payload)

	resp, err := httpClient.Post(
		"http://"+targetPeer+"/api/peers",
		"application/json",
		bytes.NewBuffer(jsonData),
	)

	if err != nil {
		// fmt.Printf("❌ Sync Failed with %s: %v\n", targetPeer, err)
		return
	}
	defer resp.Body.Close()

	var newPeers []string
	if err := json.NewDecoder(resp.Body).Decode(&newPeers); err != nil {
		return
	}

	count := 0
	for _, p := range newPeers {
		if p != myAddr {
			pm.AddPeer(p)
			count++
		}
	}
	// fmt.Printf("✅ Sync Complete. Learned %d peers from %s\n", count, targetPeer)
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
			// fmt.Printf("⚠️ Peer %s failed: %v\n", p, err)
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