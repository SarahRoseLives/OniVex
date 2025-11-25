package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"onivex/bloom"
	"onivex/filesystem"

	"github.com/cretz/bine/tor"
)

type PeerInfo struct {
	LastSeen time.Time     `json:"last_seen"`
	Filter   *bloom.Filter `json:"filter"`
}

type PeerManager struct {
	mu         sync.RWMutex
	KnownPeers map[string]PeerInfo
	Tor        *tor.Tor
	DataDir    string
}

type SearchResult struct {
	PeerID string                `json:"peer_id"`
	Files  []filesystem.FileMeta `json:"files"`
	Source string                `json:"source"`
}

func NewPeerManager(t *tor.Tor) *PeerManager {
	cwd, _ := os.Getwd()
	dataDir := filepath.Join(cwd, "data")
	os.MkdirAll(dataDir, 0700)

	pm := &PeerManager{
		KnownPeers: make(map[string]PeerInfo),
		Tor:        t,
		DataDir:    dataDir,
	}
	pm.LoadPeers()
	return pm
}

func (pm *PeerManager) AddPeer(onionAddr string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if onionAddr == "" {
		return
	}

	info, exists := pm.KnownPeers[onionAddr]
	if !exists {
		fmt.Printf("🔭 New Peer Discovered: %s\n", onionAddr)
		info = PeerInfo{}
	}
	info.LastSeen = time.Now()
	pm.KnownPeers[onionAddr] = info
}

func (pm *PeerManager) UpdatePeerFilter(onionAddr string, filter *bloom.Filter) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, exists := pm.KnownPeers[onionAddr]; exists {
		info.Filter = filter
		info.LastSeen = time.Now()
		pm.KnownPeers[onionAddr] = info
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

func (pm *PeerManager) GetRandomPeers(limit int) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	list := []string{}
	for p := range pm.KnownPeers {
		list = append(list, p)
		if len(list) >= limit {
			break
		}
	}
	return list
}

func (pm *PeerManager) LoadPeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	path := filepath.Join(pm.DataDir, "peers.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var loaded map[string]PeerInfo
	if err := json.Unmarshal(data, &loaded); err == nil {
		pm.KnownPeers = loaded
	}
}

func (pm *PeerManager) SavePeers() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if len(pm.KnownPeers) == 0 {
		return
	}
	path := filepath.Join(pm.DataDir, "peers.json")
	data, err := json.MarshalIndent(pm.KnownPeers, "", "  ")
	if err == nil {
		os.WriteFile(path, data, 0600)
	}
}

func (pm *PeerManager) StartPersistence(interval time.Duration) {
	go func() {
		for range time.Tick(interval) {
			pm.SavePeers()
		}
	}()
}

func (pm *PeerManager) StartCleanup(interval time.Duration, peerTimeout time.Duration) {
	go func() {
		for range time.Tick(interval) {
			pm.mu.Lock()
			now := time.Now()
			count := 0
			for peer, info := range pm.KnownPeers {
				if now.Sub(info.LastSeen) > peerTimeout {
					delete(pm.KnownPeers, peer)
					count++
				}
			}
			pm.mu.Unlock()
			if count > 0 {
				pm.SavePeers()
			}
		}
	}()
}

func (pm *PeerManager) Bootstrap(myOnionAddr string) {
	if len(BootstrapPeers) > 0 {
		for _, seed := range BootstrapPeers {
			if seed != myOnionAddr {
				go pm.Sync(seed, myOnionAddr)
			}
		}
	}
	pm.mu.RLock()
	candidates := make([]string, 0, len(pm.KnownPeers))
	for p := range pm.KnownPeers {
		candidates = append(candidates, p)
	}
	pm.mu.RUnlock()

	limit := 5
	for i, peer := range candidates {
		if i >= limit {
			break
		}
		if peer != myOnionAddr {
			go pm.Sync(peer, myOnionAddr)
		}
	}
}

func (pm *PeerManager) Sync(targetPeer string, myAddr string) {
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dialCancel()

	dialer, err := pm.Tor.Dialer(dialCtx, nil)
	if err != nil {
		return
	}

	client := &http.Client{
		Transport: &http.Transport{DialContext: dialer.DialContext},
		Timeout:   30 * time.Second,
	}

	payload := map[string]string{"addr": myAddr}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := client.Post("http://"+targetPeer+"/api/peers", "application/json", bytes.NewBuffer(jsonPayload))
	if err == nil {
		var newPeers []string
		if json.NewDecoder(resp.Body).Decode(&newPeers) == nil {
			for _, p := range newPeers {
				if p != myAddr {
					pm.AddPeer(p)
				}
			}
		}
		resp.Body.Close()
	}

	resp, err = client.Get("http://" + targetPeer + "/api/filter")
	if err == nil {
		var filter bloom.Filter
		if json.NewDecoder(resp.Body).Decode(&filter) == nil {
			pm.UpdatePeerFilter(targetPeer, &filter)
		}
		resp.Body.Close()
	}
}

func (pm *PeerManager) ForwardSearch(query string, ttl int, originAddr string) {
	if ttl <= 0 {
		return
	}
	peers := pm.GetRandomPeers(3)

	// FIX 1: Create Dialer ONCE before loop to prevent Control Port Deadlock
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer dialCancel()

	// If Tor is busy, we just fail gracefully instead of hanging
	dialer, err := pm.Tor.Dialer(dialCtx, nil)
	if err != nil {
		return
	}

	for _, p := range peers {
		if p == originAddr {
			continue
		}
		go func(peerID string) {
			// Reuse the existing dialer!
			client := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				Timeout:   20 * time.Second,
			}
			safeQuery := url.QueryEscape(query)
			urlStr := fmt.Sprintf("http://%s/api/query?q=%s&ttl=%d&origin=%s", peerID, safeQuery, ttl-1, originAddr)
			client.Get(urlStr)
		}(p)
	}
}

func (pm *PeerManager) SearchNetwork(query string, myAddr string) []SearchResult {
	peers := pm.GetPeers()
	fmt.Printf("🔍 Searching %d peers for '%s'...\n", len(peers), query)

	var results []SearchResult
	var mu sync.Mutex
	query = strings.ToLower(query)

	pm.mu.RLock()
	candidates := []string{}
	for peerID, info := range pm.KnownPeers {
		if peerID == myAddr {
			continue
		}
		if info.Filter != nil {
			if info.Filter.Test([]byte(query)) {
				candidates = append(candidates, peerID)
			}
		} else {
			candidates = append(candidates, peerID)
		}
	}
	pm.mu.RUnlock()

	// --- CRITICAL FIX START ---
	// Create the Tor Dialer ONCE before entering the goroutines.
	// This prevents 10 simultaneous requests to the Tor Control Port,
	// which was causing the "Write line: GETCONF" deadlock.

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer dialCancel()

	sharedDialer, err := pm.Tor.Dialer(dialCtx, nil)
	if err != nil {
		fmt.Printf("❌ Critical: Could not get Tor Dialer: %v\n", err)
		return []SearchResult{} // Return empty if Tor is dead
	}
	// --- CRITICAL FIX END ---

	maxWorkers := 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	isSeed := make(map[string]bool)
	for _, s := range BootstrapPeers {
		isSeed[s] = true
	}

	for _, p := range candidates {
		if p == myAddr { continue }
		if isSeed[p] { continue }

		wg.Add(1)
		go func(peerID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fmt.Printf("   ➡ Dialing %s...\n", peerID)

			// Reuse the sharedDialer here!
			client := &http.Client{
				Transport: &http.Transport{DialContext: sharedDialer.DialContext},
				Timeout:   45 * time.Second,
			}

			safeQuery := url.QueryEscape(query)
			resp, err := client.Get(fmt.Sprintf("http://%s/api/search?q=%s", peerID, safeQuery))
			if err != nil {
				// Don't log every timeout, it spams the console
				return
			}
			defer resp.Body.Close()

			var remoteFiles []filesystem.FileMeta
			if err := json.NewDecoder(resp.Body).Decode(&remoteFiles); err == nil && len(remoteFiles) > 0 {
				fmt.Printf("   ✅ HIT: Found %d files on %s\n", len(remoteFiles), peerID)
				mu.Lock()
				results = append(results, SearchResult{
					PeerID: peerID,
					Files:  remoteFiles,
					Source: "network",
				})
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()

	if len(results) == 0 {
		go pm.ForwardSearch(query, 2, myAddr)
	}

	return results
}