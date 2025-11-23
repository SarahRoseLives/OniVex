package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cretz/bine/tor"
)

// PeerManager handles the list of known neighbors
type PeerManager struct {
	mu         sync.RWMutex // Read-Write mutex for thread safety
	KnownPeers map[string]bool
	Tor        *tor.Tor
}

// NewPeerManager creates the address book
func NewPeerManager(t *tor.Tor) *PeerManager {
	return &PeerManager{
		KnownPeers: make(map[string]bool),
		Tor:        t,
	}
}

// AddPeer adds a neighbor to memory safely
func (pm *PeerManager) AddPeer(onionAddr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.KnownPeers[onionAddr] {
		pm.KnownPeers[onionAddr] = true
		fmt.Printf("🔭 New Peer Discovered: %s\n", onionAddr)
	}
}

// GetPeers returns a slice of all known peer addresses (for the API)
func (pm *PeerManager) GetPeers() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	list := []string{}
	for p := range pm.KnownPeers {
		list = append(list, p)
	}
	return list
}

// Sync connects to a peer and asks "Who do you know?"
func (pm *PeerManager) Sync(onionAddr string) {
	fmt.Printf("📡 Syncing with %s...\n", onionAddr)

	// 1. Create Dialer
	dialer, err := pm.Tor.Dialer(context.Background(), nil)
	if err != nil {
		fmt.Printf("❌ Dialer Error: %v\n", err)
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}

	// 2. Request their Peer List
	resp, err := httpClient.Get("http://" + onionAddr + "/api/peers")
	if err != nil {
		fmt.Printf("❌ Sync Failed (Peer might be offline): %v\n", err)
		return
	}
	defer resp.Body.Close()

	// 3. Decode JSON Response
	var newPeers []string
	if err := json.NewDecoder(resp.Body).Decode(&newPeers); err != nil {
		fmt.Printf("❌ Invalid JSON from peer: %v\n", err)
		return
	}

	// 4. Merge into our list
	count := 0
	for _, p := range newPeers {
		// Avoid adding ourselves if the peer sends our own address back
		// (Optional check, but good practice)
		pm.AddPeer(p)
		count++
	}
	fmt.Printf("✅ Sync Complete. Learned %d peers from %s\n", count, onionAddr)
}