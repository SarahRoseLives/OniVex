package network

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
)

// KeyData represents the JSON structure of the saved key
type KeyData struct {
	Type       string `json:"type"`
	PrivateKey string `json:"private_key"`
}

// LoadOrGenerateKey retrieves a key from disk or creates a new one
func LoadOrGenerateKey(name string) (ed25519.PrivateKey, error) {
	cwd, _ := os.Getwd()
	// Keys are stored in ./data/name.key
	keyPath := filepath.Join(cwd, "data", name+".key")

	// 1. Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil {
		var k KeyData
		if err := json.Unmarshal(data, &k); err == nil {
			decoded, _ := base64.StdEncoding.DecodeString(k.PrivateKey)
			return ed25519.PrivateKey(decoded), nil
		}
	}

	// 2. Generate new key if none exists
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}

	// 3. Save key to disk
	k := KeyData{
		Type:       "ed25519",
		PrivateKey: base64.StdEncoding.EncodeToString(priv),
	}
	jsonData, _ := json.MarshalIndent(k, "", "  ")

	// Ensure data dir exists
	os.MkdirAll(filepath.Join(cwd, "data"), 0700)
	os.WriteFile(keyPath, jsonData, 0600)

	return priv, nil
}