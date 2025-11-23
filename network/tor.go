package network

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cretz/bine/tor"
)

// SetupTor starts Tor.
// If keyName is provided, it persists the onion address to disk (Seed Mode).
// If keyName is empty "", it generates a fresh, temporary onion address (Client Mode).
func SetupTor(keyName string) (*tor.Tor, *tor.OnionService, error) {
	fmt.Println("🌱 Initializing Tor...")

	// 1. Data Directory
	cwd, _ := os.Getwd()
	dataDir := filepath.Join(cwd, "data", "tor")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("could not create data dir: %w", err)
	}

	// 2. Key Logic
	var privKey ed25519.PrivateKey
	var err error

	if keyName != "" {
		// SEED MODE: Load or Create & Save
		fmt.Printf("🔐 Loading persistent identity: %s\n", keyName)
		privKey, err = LoadOrGenerateKey(keyName)
	} else {
		// CLIENT MODE: Generate Ephemeral Key (No Save)
		fmt.Println("👻 Generating temporary anonymous identity...")
		_, privKey, err = ed25519.GenerateKey(nil)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("key generation failed: %w", err)
	}

	// 3. Start Tor
	conf := &tor.StartConf{
		DataDir:     dataDir,
		DebugWriter: os.Stdout,
	}

	// Using nil for config to download Tor executable automatically if not present
	t, err := tor.Start(nil, conf)
	if err != nil {
		return nil, nil, fmt.Errorf("tor start failed: %w", err)
	}

	// 4. Create Onion Service
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("🧅 Creating/Restoring V3 Onion Service...")

	onion, err := t.Listen(ctx, &tor.ListenConf{
		Version3:    true,
		RemotePorts: []int{80},
		Key:         privKey,
	})

	return t, onion, err
}