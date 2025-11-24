package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"onivex/filesystem"

	"github.com/cretz/bine/tor"
)

type PeerInfo struct {
	LastSeen time.Time             `json:"last_seen"`
	Files    []filesystem.FileMeta `json:"files"`
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
	if onionAddr == "" { return }

	info, exists := pm.KnownPeers[onionAddr]
	if !exists {
		fmt.Printf("🔭 New Peer Discovered: %s\n", onionAddr)
		info = PeerInfo{Files: []filesystem.FileMeta{}}
	}
	info.LastSeen = time.Now()
	pm.KnownPeers[onionAddr] = info
}

func (pm *PeerManager) UpdatePeerIndex(onionAddr string, files []filesystem.FileMeta) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, exists := pm.KnownPeers[onionAddr]; exists {
		info.Files = files
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

func (pm *PeerManager) LoadPeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	path := filepath.Join(pm.DataDir, "peers.json")
	data, err := os.ReadFile(path)
	if err != nil { return }
	var loaded map[string]PeerInfo
	if err := json.Unmarshal(data, &loaded); err == nil {
		pm.KnownPeers = loaded
	}
}

func (pm *PeerManager) SavePeers() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if len(pm.KnownPeers) == 0 { return }
	path := filepath.Join(pm.DataDir, "peers.json")
	data, err := json.MarshalIndent(pm.KnownPeers, "", "  ")
	if err == nil { os.WriteFile(path, data, 0600) }
}

func (pm *PeerManager) StartPersistence(interval time.Duration) {
	go func() {
		for range time.Tick(interval) { pm.SavePeers() }
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
			if count > 0 { pm.SavePeers() }
		}
	}()
}

// --- NETWORK ---

func (pm *PeerManager) Bootstrap(myOnionAddr string) {
	if len(BootstrapPeers) > 0 {
		for _, seed := range BootstrapPeers {
			if seed != myOnionAddr { go pm.Sync(seed, myOnionAddr) }
		}
	}
	pm.mu.RLock()
	candidates := make([]string, 0, len(pm.KnownPeers))
	for p := range pm.KnownPeers { candidates = append(candidates, p) }
	pm.mu.RUnlock()

	limit := 5
	for i, peer := range candidates {
		if i >= limit { break }
		if peer != myOnionAddr { go pm.Sync(peer, myOnionAddr) }
	}
}

func (pm *PeerManager) Sync(targetPeer string, myAddr string) {
	dialer, err := pm.Tor.Dialer(context.Background(), nil)
	if err != nil { return }

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
				if p != myAddr { pm.AddPeer(p) }
			}
		}
		resp.Body.Close()
	}

	resp, err = client.Get("http://" + targetPeer + "/api/index")
	if err == nil {
		var files []filesystem.FileMeta
		if json.NewDecoder(resp.Body).Decode(&files) == nil {
			if len(files) > 0 { pm.UpdatePeerIndex(targetPeer, files) }
		}
		resp.Body.Close()
	}
}

// SearchNetwork performs the Parallel Search
func (pm *PeerManager) SearchNetwork(query string, myAddr string) []SearchResult {
	peers := pm.GetPeers()
	fmt.Printf("🔍 Searching %d peers for '%s'...\n", len(peers), query)

	var results []SearchResult
	var mu sync.Mutex
	query = strings.ToLower(query)

	// 1. Local Cache
	pm.mu.RLock()
	for peerID, info := range pm.KnownPeers {
		if peerID == myAddr { continue }
		var matches []filesystem.FileMeta
		for _, f := range info.Files {
			if strings.Contains(strings.ToLower(f.Name), query) {
				matches = append(matches, f)
			}
		}
		if len(matches) > 0 {
			results = append(results, SearchResult{
				PeerID: peerID,
				Files:  matches,
				Source: "cache",
			})
		}
	}
	pm.mu.RUnlock()

	// 2. Live Network Search
	maxWorkers := 10
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	isSeed := make(map[string]bool)
	for _, s := range BootstrapPeers { isSeed[s] = true }

	for _, p := range peers {
		if p == myAddr { continue }
		if isSeed[p] { continue }

		wg.Add(1)
		go func(peerID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dialer, err := pm.Tor.Dialer(context.Background(), nil)
			if err != nil { return }

			client := &http.Client{
				Transport: &http.Transport{DialContext: dialer.DialContext},
				// UPDATED: Increased timeout to 20s for initial connection
				Timeout:   20 * time.Second,
			}

			url := fmt.Sprintf("http://%s/api/search?q=%s", peerID, query)
			// DEBUG PRINT
			fmt.Printf("   ➡ Dialing %s...\n", peerID)

			resp, err := client.Get(url)
			if err != nil {
				// DEBUG PRINT ERROR
				fmt.Printf("   ❌ Failed %s: %v\n", peerID, err)
				return
			}
			defer resp.Body.Close()

			var remoteFiles []filesystem.FileMeta
			if err := json.NewDecoder(resp.Body).Decode(&remoteFiles); err == nil && len(remoteFiles) > 0 {
				fmt.Printf("   ✅ Found %d files on %s\n", len(remoteFiles), peerID)
				mu.Lock()
				results = append(results, SearchResult{
					PeerID: peerID,
					Files:  remoteFiles,
					Source: "network",
				})
				mu.Unlock()
				pm.UpdatePeerIndex(peerID, remoteFiles)
			} else {
				fmt.Printf("   ⚪ No matches on %s\n", peerID)
			}
		}(p)
	}

	wg.Wait()
	return results
}