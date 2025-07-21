package wallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/gcm"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// make a reproducible test AppLinkInfo
func testInfo() *AppLinkInfo {
	now := time.Now().Unix()
	return &AppLinkInfo{
		AppName:   "unit-test",
		CreatedAt: now,
		ExpireAt:  now + int64(24*time.Hour/time.Second),
		Scope:     42,
	}
}

func equalInfos(a, b *AppLinkInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	// we ignore AppHash because it is filled only on read
	return a.AppName == b.AppName &&
		a.CreatedAt == b.CreatedAt &&
		a.Scope == b.Scope
}

func TestGenerateLoad_RoundTrip_V1(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// payload for v1
	want := testInfo()

	info, err := generate(dir, pk, want.AppName, model.AccountAuthLocalApiScope(want.Scope))
	if err != nil {
		t.Fatalf("Generate(v1): %v", err)
	}
	gotV1, err := load(dir, info.AppKey, pk)
	if err != nil {
		t.Fatalf("Load(v1): %v", err)
	}
	if !equalInfos(want, gotV1) {
		t.Fatalf("v1 payload mismatch.\nwant: %+v\ngot : %+v", want, gotV1)
	}
}

func TestGenerateLoad_RoundTrip_V0(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	// Make sure the directory exists
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// payload for v0
	want := testInfo()

	// ───────────────────────── v0 round-trip ─────────────
	appKeyV0, err := writeAppLinkFileV0(pk, dir, want)
	if err != nil {
		t.Fatalf("writeAppLinkFileV0: %v", err)
	}
	gotV0, err := load(dir, appKeyV0, pk)
	if err != nil {
		t.Fatalf("Load(v0): %v", err)
	}
	if !equalInfos(want, gotV0) {
		t.Fatalf("v0 payload mismatch.\nwant: %+v\ngot : %+v", want, gotV0)
	}
}

func TestNoDuplicateKeys_V0_V1(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	// Make sure the directory exists
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// common payload for both versions
	want := testInfo()

	// Generate keys for both v0 and v1
	appKeyV1, err := generate(dir, pk, want.AppName, model.AccountAuthLocalApiScope(want.Scope))
	require.NoError(t, err)

	appKeyV0, err := writeAppLinkFileV0(pk, dir, want)
	require.NoError(t, err)

	// ───────────────────────── sanity: v0≠v1 keys ────────
	if reflect.DeepEqual(appKeyV0, appKeyV1) {
		t.Fatalf("appKey collision between v0 and v1 – should never happen")
	}
}

func TestList(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	// Make sure the directory exists
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// Create v1 entry
	info1 := &AppLinkInfo{
		AppName:   "app1",
		CreatedAt: time.Now().Unix(),
		ExpireAt:  time.Now().Unix() + 3600,
		Scope:     1,
	}
	info, err := generate(dir, pk, info1.AppName, model.AccountAuthLocalApiScope(info1.Scope))
	require.NoError(t, err)

	// Create v0 entry
	info2 := &AppLinkInfo{
		AppName:   "app2",
		CreatedAt: time.Now().Unix(),
		ExpireAt:  time.Now().Unix() + 3600,
		Scope:     2,
	}
	appKey2, err := writeAppLinkFileV0(pk, dir, info2)
	require.NoError(t, err)

	// List all entries (with account key)
	entries, err := list(dir, pk)
	require.NoError(t, err)
	require.Equal(t, 2, len(entries), "should have found 2 entries")

	// Create maps for easier lookup by AppName
	entriesByName := make(map[string]*AppLinkInfo)
	for _, entry := range entries {
		if entry.AppName != "" {
			entriesByName[entry.AppName] = entry
		}
	}

	// Verify v1 entry has full info
	app1Entry, found := entriesByName["app1"]
	require.True(t, found, "should have found app1 entry")
	require.Equal(t, info1.Scope, app1Entry.Scope)
	require.NotEmpty(t, app1Entry.AppHash, "v1 entry should have AppHash")

	// Also verify we can load this entry explicitly with the key
	loaded, err := load(dir, info.AppKey, pk)
	require.NoError(t, err)
	require.Equal(t, info1.AppName, loaded.AppName)

	// Verify v0 entries have AppHash but not other fields
	// Since v0 doesn't populate AppName when listing, we need to count entries
	// with non-empty AppHash but empty AppName
	count := 0
	var v0Entry *AppLinkInfo
	for _, entry := range entries {
		if entry.AppName == "" && entry.AppHash != "" {
			count++
			v0Entry = entry
		}
	}
	require.Equal(t, 1, count, "should have found 1 v0 entry")
	require.NotNil(t, v0Entry, "should have a v0 entry")

	// Verify we can load the v0 entry explicitly with the key
	loaded, err = load(dir, appKey2, pk)
	require.NoError(t, err)
	require.Equal(t, info2.AppName, loaded.AppName)

	// Test listing without account key
	entriesNoKey, err := list(dir, nil)
	require.NoError(t, err)
	require.Equal(t, 2, len(entriesNoKey), "should have found 2 entries")

	// All entries should have AppHash but no other fields
	for _, entry := range entriesNoKey {
		require.NotEmpty(t, entry.AppHash, "entry should have AppHash")
		require.Empty(t, entry.AppName, "entry should not have AppName without key")
	}
}

func TestRevoke(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	// Make sure the directory exists
	err := os.MkdirAll(dir, 0o700)
	require.NoError(t, err)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// Create an app link
	info := &AppLinkInfo{
		AppName:   "test-app",
		CreatedAt: time.Now().Unix(),
		ExpireAt:  time.Now().Unix() + 3600,
		Scope:     1,
	}

	// Generate the app link file
	_, err = generate(dir, pk, info.AppName, model.AccountAuthLocalApiScope(info.Scope))
	require.NoError(t, err)

	// List entries to get the app hash
	entries, err := list(dir, pk)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries), "should have one entry")

	appHash := entries[0].AppHash
	require.NotEmpty(t, appHash, "app hash should not be empty")

	// Verify the file exists
	filePath := filepath.Join(dir, appHash+".json")
	_, err = os.Stat(filePath)
	require.NoError(t, err, "file should exist")

	// Test revoke for existing file
	err = revoke(dir, appHash)
	require.NoError(t, err, "should successfully revoke existing file")

	// Verify the file no longer exists
	_, err = os.Stat(filePath)
	require.True(t, os.IsNotExist(err), "file should not exist after revocation")

	// Test revoke for non-existent file
	err = revoke(dir, "nonexistent-hash")
	require.Equal(t, ErrAppLinkNotFound, err, "should return ErrAppLinkNotFound for non-existent file")

	// Test revoke with empty hash
	err = revoke(dir, "")
	require.Equal(t, ErrAppLinkNotFound, err, "should return ErrAppLinkNotFound for empty hash")
}

// writeAppLinkFileV0: legacy support for v0 files. Used for tests
func writeAppLinkFileV0(accountPk crypto.PrivKey, dir string, payload *AppLinkInfo) (appKey string, err error) {
	key, err := symmetric.NewRandom()
	if err != nil {
		return
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return
	}

	encPayloadReader, err := gcm.New(key).EncryptReader(bytes.NewReader(b))
	if err != nil {
		return "", err
	}

	encPayload, err := io.ReadAll(encPayloadReader)
	if err != nil {
		return "", err
	}

	appKey = base64.StdEncoding.EncodeToString(key.Bytes())
	signature, err := accountPk.Sign(encPayload)

	if err != nil {
		return "", err
	}
	encryptedPayload := fileV0{
		Payload:   encPayload,
		Signature: signature,
	}

	hash := sha256.Sum256(key.Bytes())
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%x", hash)+".json"))
	if err != nil {
		return "", fmt.Errorf("failed to create app key file in the account: %w", err)
	}

	w := json.NewEncoder(f)
	err = w.Encode(encryptedPayload)
	if err != nil {
		return "", err
	}
	return appKey, nil
}
