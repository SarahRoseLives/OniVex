# 👻 Onivex

The Resilient, Anonymous, Unstructured P2P Mesh Network.

Onivex is a file-sharing application built on top of Tor Hidden Services. It creates a fully decentralized, censorship-resistant mesh network where every peer is both a client and a server. Unlike many traditional privacy-preserving tools that are slow and hard to discover, Onivex uses Gossip Protocols, Bloom Filters, and TTL Flooding to make searching privacy-preserving networks fast and bandwidth-efficient.

---

## 🚀 Key Features

- 🧅 **Total Anonymity**  
  All traffic is routed through Tor. IPs are never exposed.

- 🕸️ **Decentralized Mesh**  
  No central server. If the seeds go down, the network survives via peer memory.

- ⚡ **Efficient Search**  
  Uses Bloom Filters to index thousands of files with minimal bandwidth and Parallel Fan-Out to query nodes simultaneously.

- 💾 **Persistent Identity**  
  Your node remembers its identity and its friends. Restarting the app doesn't sever your ties to the mesh.

- 🔍 **Deep Search**  
  Uses Query Forwarding (TTL Flooding) to find files on nodes you aren't directly connected to.

---

## Table of Contents

- [How to Use (Client)](#how-to-use-client)
  - [Starting the Client](#starting-the-client)
  - [Sharing Files](#sharing-files)
  - [Searching & Downloading](#searching--downloading)
  - [Multiple Instances (Testing)](#multiple-instances-testing)
- [🌱 Running a Seed Node (Bootstrapper)](#-running-a-seed-node-bootstrapper)
  - [Why run a seed?](#why-run-a-seed)
  - [Compiling & Running the Seed](#compiling--running-the-seed)
- [🧠 Architecture Details](#-architecture-details)
  - [The "Ghost Peer" Solution](#the-ghost-peer-solution)
  - [Search Horizon](#search-horizon)
- [⚠️ Disclaimer](#️-disclaimer)
- [Contributing](#contributing)
- [License](#license)

---

## 💻 How to Use (Client)

### 1. Starting the Client

Onivex runs as a background daemon with a web-based interface.

- Run the binary (or `go run client/main.go`).
- Wait 15–30 seconds. Tor needs time to build circuits and establish your Hidden Service.
- The console will display your local control URL.

```plaintext
✨ ONIVEX CLIENT LIVE
👉 Tor Access: http://vtwod4...xyz.onion
👉 Control UI: http://127.0.0.1:8080
```

Open `http://127.0.0.1:8080` in your standard browser (Chrome / Firefox / Edge).

### 2. Sharing Files

Onivex shares files located in the `uploads/` directory created next to the binary.

- To share: simply drop files into the `uploads/` folder.
- To index: the app automatically rescans this folder and updates your Bloom Filter for the network.

Files you put in `uploads/` will be advertised to peers according to the network's gossip and Bloom filter propagation.

### 3. Searching & Downloading

- Search: Go to the Search tab. Type a keyword (e.g., `linux`, `book`).

  Note: Results from your local disk appear instantly. Results from the network will trickle in over 5–10 seconds as the query propagates through Tor.

- Download: Click the Download button next to a file.

  The file will be proxied through the Onivex daemon (tunneling through Tor) and saved to your `downloads/` folder.

### 4. Multiple Instances (Testing)

You can run multiple independent nodes on the same machine for testing. Run instances from different folders (each keeps its own data directory).

```bash
# Node A
./onivex -port 8080

# Node B (Run from a separate folder!)
./onivex -port 8081
```

---

## 🌱 Running a Seed Node (Bootstrapper)

The Seed Node is a lightweight, headless server designed to help new users find the network. It does not host files; it only introduces peers to one another.

### Why run a seed?

While the network can survive without seeds (using local peer memory), a Seed Node provides a reliable entry point for brand new installations.

### Compiling & Running the Seed

Build the seed binary:

```bash
go build -o seed bootstrap/main.go
```

Run it on a VPS or other stable server:

```bash
./seed
```

Important: On the first run, the seed will generate a persistent identity. Look for this output:

```plaintext
⭐ SEED ADDRESS (Copy to discovery/bootstrap.go): 
   vtwod4...xyz.onion
```

To make this seed the default for your custom build, paste that address into `discovery/bootstrap.go` in the `BootstrapPeers` list.

---

## 🧠 Architecture Details

### The "Ghost Peer" Solution

Onivex uses Persistent Identities. When you start the client, it generates a cryptographic key pair stored at:

```
data/client_identity.key
```

- This ensures your Onion Address remains the same between restarts.
- Other peers can "remember" you, keeping the mesh connected even if seeds go offline.

### Search Horizon

Onivex uses a Hybrid Search mechanism:

- **Local Index**: Checks a local cache of Bloom Filters from peers you have recently synced with (instant).
- **Direct Search**: Actively queries live peers in parallel.
- **Flooding**: If the file isn't found, your peers forward the query to their peers (TTL=2), expanding your reach exponentially.

These mechanisms combined provide fast, bandwidth-efficient searching over Tor.

---

## ⚠️ Disclaimer

Onivex is a tool for uncensored communication and file sharing.

- Anonymity is not magic: While Tor hides your IP, your behavior (file choices, uptime) can still fingerprint you.
- Tor Performance: Searching over Tor is slower than the open web. Be patient.
- Use Responsibly: The developers are not responsible for the content shared over the network.

