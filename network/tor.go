package network

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cretz/bine/tor"
)

// SetupTor starts a managed Tor instance
// UPDATED: Now returns (*tor.Tor, *tor.OnionService, error)
func SetupTor() (*tor.Tor, *tor.OnionService, error) {
	fmt.Println("🌱 Initializing Tor...")

	// 1. Get Absolute Path for Data Directory
	cwd, _ := os.Getwd()
	dataDir := filepath.Join(cwd, "data", "tor")

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("could not create data dir: %w", err)
	}

	fmt.Printf("📂 Using Data Dir: %s\n", dataDir)

	// 2. Configure Tor
	conf := &tor.StartConf{
		DataDir:     dataDir,
		DebugWriter: os.Stdout,
	}

	fmt.Println("🚀 Starting Tor background process...")
	t, err := tor.Start(nil, conf)
	if err != nil {
		return nil, nil, fmt.Errorf("tor start failed: %w", err)
	}

	// 3. Create the Onion Service
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fmt.Println("🧅 Creating V3 Onion Service...")

	onion, err := t.Listen(ctx, &tor.ListenConf{
		Version3:    true,
		RemotePorts: []int{80},
	})
	if err != nil {
		t.Close()
		return nil, nil, fmt.Errorf("listen failed: %w", err)
	}

	// RETURN BOTH: The Controller (t) and the Service (onion)
	return t, onion, nil
}