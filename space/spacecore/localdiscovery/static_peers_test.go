//go:build !android
// +build !android

package localdiscovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadStaticPeers_ReadsAndCaches(t *testing.T) {
	l := &localDiscovery{repoPath: t.TempDir()}

	// no file: nothing loaded, not changed
	peers, changed := l.loadStaticPeers()
	assert.False(t, changed)
	assert.Nil(t, peers)

	require.NoError(t, os.WriteFile(filepath.Join(l.repoPath, "static-peers.json"), []byte(`[
		{"peerId":"12D3KooAAA","addresses":["10.0.0.5:41637"]},
		{"peerId":"12D3KooBBB","addresses":["10.0.0.6:41637","10.0.0.6:41638"]}
	]`), 0640))

	peers, changed = l.loadStaticPeers()
	require.True(t, changed)
	require.Len(t, peers, 2)
	assert.Equal(t, "12D3KooAAA", peers[0].PeerId)
	assert.Equal(t, []string{"10.0.0.5:41637"}, peers[0].Addresses)
	assert.Len(t, peers[1].Addresses, 2)

	// second read with no mtime change: cached, not changed
	peers, changed = l.loadStaticPeers()
	assert.False(t, changed)
	assert.Len(t, peers, 2)
}

func TestLoadStaticPeers_RereadsOnMtimeChange(t *testing.T) {
	l := &localDiscovery{repoPath: t.TempDir()}
	path := filepath.Join(l.repoPath, "static-peers.json")

	require.NoError(t, os.WriteFile(path, []byte(`[{"peerId":"A","addresses":["1.1.1.1:5"]}]`), 0640))
	_, changed := l.loadStaticPeers()
	require.True(t, changed)

	// ensure mtime moves forward even on fast filesystems
	newMtime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(path, newMtime, newMtime))
	require.NoError(t, os.WriteFile(path, []byte(`[{"peerId":"B","addresses":["2.2.2.2:6"]}]`), 0640))
	require.NoError(t, os.Chtimes(path, newMtime.Add(time.Second), newMtime.Add(time.Second)))

	peers, changed := l.loadStaticPeers()
	require.True(t, changed)
	require.Len(t, peers, 1)
	assert.Equal(t, "B", peers[0].PeerId)
}

func TestStaticPeerAddrs_StripsSchemeAndDropsEmpty(t *testing.T) {
	addrs := staticPeerAddrs([]string{
		"10.0.0.5:41637",
		"yamux://10.0.0.6:41637",
		"quic://10.0.0.7:41637",
		"",
	})
	// addSchema pins the transport, so addresses must arrive as bare host:port
	// regardless of the form the user pasted from own-address.json
	assert.Equal(t, []string{"10.0.0.5:41637", "10.0.0.6:41637", "10.0.0.7:41637"}, addrs)
	assert.Empty(t, staticPeerAddrs(nil))
}

func TestWriteOwnAddress_StaticPeersEntryFormat(t *testing.T) {
	repo := t.TempDir()
	l := &localDiscovery{repoPath: repo, peerId: "12D3KooTEST"}

	l.writeOwnAddress(OwnAddresses{Addrs: []string{"192.168.1.5", "10.0.0.5"}, Port: 41637})

	// own-address.json must be parseable as static-peers.json content as-is,
	// with no redundant fields beyond the entry itself
	raw, err := os.ReadFile(filepath.Join(repo, "own-address.json"))
	require.NoError(t, err)
	var entries []staticPeer
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "12D3KooTEST", entries[0].PeerId)
	assert.Equal(t, []string{"yamux://192.168.1.5:41637", "yamux://10.0.0.5:41637"}, entries[0].Addresses)
	assert.NotContains(t, string(raw), "\"note\"")
	assert.NotContains(t, string(raw), "\"port\"")

	// unchanged content must not be rewritten
	fi := statOwnAddr(t, repo)
	l.writeOwnAddress(OwnAddresses{Addrs: []string{"192.168.1.5", "10.0.0.5"}, Port: 41637})
	assert.Equal(t, fi.ModTime(), statOwnAddr(t, repo).ModTime())
}

func statOwnAddr(t *testing.T, repo string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(filepath.Join(repo, "own-address.json"))
	require.NoError(t, err)
	return fi
}

func TestLoadStaticPeers_InvalidJSONIgnored(t *testing.T) {
	l := &localDiscovery{repoPath: t.TempDir()}
	require.NoError(t, os.WriteFile(filepath.Join(l.repoPath, "static-peers.json"), []byte(`not json`), 0640))

	peers, changed := l.loadStaticPeers()
	assert.False(t, changed)
	assert.Nil(t, peers)
}
