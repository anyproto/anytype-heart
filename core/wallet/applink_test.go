package wallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
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
		a.ExpireAt == b.ExpireAt &&
		a.Scope == b.Scope &&
		reflect.DeepEqual(a.Grant, b.Grant)
}

func TestGenerateLoad_RoundTrip_V1(t *testing.T) {
	// ── arrange keys & temp dir ──────────────────────────
	tmp := t.TempDir()
	dir := filepath.Join(tmp, appLinkKeysDirectory)

	pk, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	// payload for v1
	want := testInfo()

	info, err := generate(dir, pk, want.AppName, model.AccountAuthLocalApiScope(want.Scope), want.ExpireAt, nil)
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
	// AppHash is not stored in the sealed payload; load must re-derive it —
	// session revocation depends on the read path returning the same hash the
	// generate path produced.
	require.Equal(t, info.AppHash, gotV1.AppHash)
	require.NotEmpty(t, gotV1.AppHash)
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
	// legacy v0 files must get the re-derived AppHash on read too
	keyBytes, err := base64.StdEncoding.DecodeString(appKeyV0)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(keyBytes)), gotV0.AppHash)
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
	appKeyV1, err := generate(dir, pk, want.AppName, model.AccountAuthLocalApiScope(want.Scope), want.ExpireAt, nil)
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
	info, err := generate(dir, pk, info1.AppName, model.AccountAuthLocalApiScope(info1.Scope), info1.ExpireAt, nil)
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
	_, err = generate(dir, pk, info.AppName, model.AccountAuthLocalApiScope(info.Scope), info.ExpireAt, nil)
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

func TestReadAppLink_Expiry(t *testing.T) {
	newTestWallet := func(t *testing.T) (*wallet, string) {
		t.Helper()
		tmp := t.TempDir()
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		return &wallet{repoPath: tmp, accountKey: pk}, tmp
	}

	tests := []struct {
		name     string
		expireAt int64
		wantErr  error
	}{
		{
			name:     "expired key is refused",
			expireAt: time.Now().Unix() - 60,
			wantErr:  ErrAppLinkExpired,
		},
		{
			name:     "future expiry still loads",
			expireAt: time.Now().Unix() + 3600,
			wantErr:  nil,
		},
		{
			name:     "zero expiry never expires",
			expireAt: 0,
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			w, _ := newTestWallet(t)
			info, err := w.PersistAppLink("expiry-test", model.AccountAuth_JsonAPI, tt.expireAt, nil)
			require.NoError(t, err)

			// when
			got, err := w.ReadAppLink(info.AppKey)

			// then
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expireAt, got.ExpireAt)
				require.Equal(t, "expiry-test", got.AppName)
			}
		})
	}

	t.Run("read app link returns the app hash", func(t *testing.T) {
		// given: the application layer keys session revocation off
		// ReadAppLink().AppHash — pin the invariant where it lives
		w, _ := newTestWallet(t)
		info, err := w.PersistAppLink("hash-test", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)
		require.NotEmpty(t, info.AppHash)

		// when
		got, err := w.ReadAppLink(info.AppKey)

		// then
		require.NoError(t, err)
		require.Equal(t, info.AppHash, got.AppHash)
	})

	t.Run("expired key is still listed", func(t *testing.T) {
		// given: the file stays on disk so the user can see and revoke it
		w, tmp := newTestWallet(t)
		_, err := w.PersistAppLink("expired-but-listed", model.AccountAuth_JsonAPI, time.Now().Unix()-60, nil)
		require.NoError(t, err)

		// when
		entries, err := list(filepath.Join(tmp, appLinkKeysDirectory), w.accountKey)

		// then
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, "expired-but-listed", entries[0].AppName)
	})
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

// writeLegacyV1AppLink replicates what generate() produced before the key
// format flip and before grants existed: a raw base64 key string inside a
// ver-1 envelope. Tests use it to pin that the existing key population — and
// its in-place upgrade path — keeps working.
func writeLegacyV1AppLink(dir string, accountPriv crypto.PrivKey, info *AppLinkInfo) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil && !os.IsExist(err) {
		return "", err
	}
	key, err := crypto.NewRandomAES()
	if err != nil {
		return "", err
	}
	filled := *info
	filled.AppKey = base64.StdEncoding.EncodeToString(key.Bytes())
	file, err := buildEnvelope(ver1, key.Bytes(), accountPriv, &filled)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%x.json", sha256.Sum256(key.Bytes()))
	fp, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer fp.Close()
	return filled.AppKey, json.NewEncoder(fp).Encode(&file)
}

// readV1OnlyEnvelopeVersion mirrors the version gate of the pre-grant reader:
// envelope versions 0 and 1 were the only known ones, everything else was
// refused with "unsupported version". The helper returns that reader's error
// for the given file so tests can assert a downgraded binary FAILS CLOSED on
// a scoped key.
func readV1OnlyEnvelopeVersion(t *testing.T, dir, appHash string) error {
	t.Helper()
	fp, err := os.Open(filepath.Join(dir, appHash+".json"))
	require.NoError(t, err)
	defer fp.Close()
	var peek struct {
		Version int `json:"ver"`
	}
	require.NoError(t, json.NewDecoder(fp).Decode(&peek))
	switch peek.Version {
	case 0, 1:
		return nil
	default:
		return fmt.Errorf("unsupported version %d", peek.Version)
	}
}

func testGrant() *AppLinkGrant {
	return &AppLinkGrant{
		Version: appLinkGrantVersion,
		Spaces:  []string{"space1", "space2"},
		Perms:   AppLinkPermsReadWrite,
	}
}

func envelopeVersionOnDisk(t *testing.T, dir, appHash string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, appHash+".json"))
	require.NoError(t, err)
	var peek struct {
		Version int `json:"ver"`
	}
	require.NoError(t, json.Unmarshal(raw, &peek))
	return peek.Version
}

func TestAppKeyFormat(t *testing.T) {
	t.Run("new format is pinned", func(t *testing.T) {
		// given: the format is a public contract — secret scanners and the
		// docs depend on it — so a change must fail a test, not slip through
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)

		// when
		info, err := generate(dir, pk, "fmt-pin", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		// then: anytype_<52 chars of lowercase unpadded base32>_<8 hex chars>.
		// The body alphabet is alphanumeric-only by hard requirement (spec P1
		// §1b): `+`, `/` and `=` break \b-anchored scanner rules, form
		// encoding, and RFC 6750 token68.
		require.Regexp(t, `^anytype_[a-z2-7]{52}_[0-9a-f]{8}$`, info.AppKey)
		// and it must stay inside the published scanner pattern, which is a
		// length RANGE so third-party rules survive a future body encoding
		require.Regexp(t, `^anytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}$`, info.AppKey)
	})

	t.Run("checksum covers the prefix", func(t *testing.T) {
		// given: spec P1 §1b — the CRC input is prefix + "_" + body, not the
		// body alone, so a mangled prefix is caught too
		body := "abcdefghijklmnopqrstuvwxyz234567abcdefghijklmnopqrst"

		// when
		got := appKeyChecksum(body)

		// then
		want := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte("anytype_"+body)))
		require.Equal(t, want, got)
	})

	t.Run("json api keys mint the new format and round-trip", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)

		// when
		info, err := generate(dir, pk, "new-format", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		// then
		got, err := load(dir, info.AppKey, pk)
		require.NoError(t, err)
		require.Equal(t, "new-format", got.AppName)
		require.Equal(t, info.AppHash, got.AppHash)
	})

	t.Run("limited keys keep the raw base64 format", func(t *testing.T) {
		// given: Limited is a gRPC/clipper credential, not a JSON-API key —
		// the prefix would falsely brand it as one (spec P1 §1b)
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)

		// when
		info, err := generate(dir, pk, "clipper", model.AccountAuth_Limited, 0, nil)
		require.NoError(t, err)

		// then
		require.NotContains(t, info.AppKey, appKeySeparator)
		raw, err := base64.StdEncoding.DecodeString(info.AppKey)
		require.NoError(t, err)
		require.Len(t, raw, 32)

		got, err := load(dir, info.AppKey, pk)
		require.NoError(t, err)
		require.Equal(t, "clipper", got.AppName)
	})

	t.Run("old format key still resolves", func(t *testing.T) {
		// given: a JsonAPI key written exactly as the pre-flip code wrote it —
		// the existing key population must keep authenticating unchanged
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		want := &AppLinkInfo{
			AppName:   "legacy-format",
			CreatedAt: time.Now().Unix(),
			Scope:     int(model.AccountAuth_JsonAPI),
		}
		appKey, err := writeLegacyV1AppLink(dir, pk, want)
		require.NoError(t, err)

		// when
		got, err := load(dir, appKey, pk)

		// then
		require.NoError(t, err)
		require.True(t, equalInfos(want, got), "want %+v got %+v", want, got)
	})

	t.Run("checksum rejection", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		info, err := generate(dir, pk, "typo-test", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		flip := func(c byte) byte {
			if c == 'a' {
				return 'b'
			}
			return 'a'
		}

		tests := []struct {
			name   string
			mangle func(key string) string
		}{
			{
				name: "typo in the checksum",
				mangle: func(key string) string {
					return key[:len(key)-1] + string(flip(key[len(key)-1]))
				},
			},
			{
				name: "typo in the body",
				mangle: func(key string) string {
					i := len(appKeyPrefix) + 1
					return key[:i] + string(flip(key[i])) + key[i+1:]
				},
			},
			{
				name: "missing checksum segment",
				mangle: func(key string) string {
					return key[:strings.LastIndex(key, appKeySeparator)]
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// when
				_, err := load(dir, tt.mangle(info.AppKey), pk)

				// then: rejected, and NOT as file-not-found — the parse
				// failed before any disk lookup
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrAppLinkNotFound)
			})
		}
	})
}

func TestGrantRoundTrip(t *testing.T) {
	t.Run("grant survives the sealed envelope", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		want := testGrant()

		// when
		info, err := generate(dir, pk, "granted", model.AccountAuth_JsonAPI, 0, want)
		require.NoError(t, err)
		got, err := load(dir, info.AppKey, pk)

		// then
		require.NoError(t, err)
		require.Equal(t, want, got.Grant)
	})

	t.Run("granted key writes envelope ver 2, unscoped keeps ver 1", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)

		// when
		granted, err := generate(dir, pk, "granted", model.AccountAuth_JsonAPI, 0, testGrant())
		require.NoError(t, err)
		unscoped, err := generate(dir, pk, "unscoped", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		// then
		assert.Equal(t, ver2, envelopeVersionOnDisk(t, dir, granted.AppHash))
		assert.Equal(t, ver1, envelopeVersionOnDisk(t, dir, unscoped.AppHash))
	})

	t.Run("a ver-1-only reader refuses a granted key", func(t *testing.T) {
		// given: the downgrade fail-closed property — an older heart binary
		// must REFUSE a scoped key, never silently honor it unscoped
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		info, err := generate(dir, pk, "granted", model.AccountAuth_JsonAPI, 0, testGrant())
		require.NoError(t, err)

		// when
		err = readV1OnlyEnvelopeVersion(t, dir, info.AppHash)

		// then
		require.ErrorContains(t, err, "unsupported version 2")
	})

	t.Run("unknown envelope versions are still refused", func(t *testing.T) {
		// given: this hard-check IS the fail-closed mechanism — if it ever
		// softens, future scoped formats stop being downgrade-safe. Build a
		// ver-3 file whose signature and HMAC genuinely verify, so the only
		// thing refusing it is the version gate.
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		appKey := base64.StdEncoding.EncodeToString(key.Bytes())
		file, err := buildEnvelope(3, key.Bytes(), pk, &AppLinkInfo{AppKey: appKey, AppName: "future", Scope: int(model.AccountAuth_JsonAPI)})
		require.NoError(t, err)
		raw, err := json.Marshal(&file)
		require.NoError(t, err)
		name := fmt.Sprintf("%x.json", sha256.Sum256(key.Bytes()))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0o600))

		// when
		_, err = load(dir, appKey, pk)

		// then
		require.ErrorContains(t, err, "unsupported version 3")
	})

	t.Run("a ver-1 envelope carrying a grant is refused", func(t *testing.T) {
		// given: grant ⇒ ver 2 must be total — in a ver-1 envelope an older
		// binary would read the key and silently drop the grant
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		appKey := base64.StdEncoding.EncodeToString(key.Bytes())
		file, err := buildEnvelope(ver1, key.Bytes(), pk, &AppLinkInfo{
			AppKey: appKey,
			Scope:  int(model.AccountAuth_JsonAPI),
			Grant:  testGrant(),
		})
		require.NoError(t, err)
		raw, err := json.Marshal(&file)
		require.NoError(t, err)
		name := fmt.Sprintf("%x.json", sha256.Sum256(key.Bytes()))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0o600))

		// when
		_, err = load(dir, appKey, pk)

		// then
		require.ErrorContains(t, err, "must not carry a grant")
	})

	t.Run("list shows the grant", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		want := testGrant()
		_, err = generate(dir, pk, "granted", model.AccountAuth_JsonAPI, 0, want)
		require.NoError(t, err)

		// when
		entries, err := list(dir, pk)

		// then
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, want, entries[0].Grant)
	})
}

func TestValidateAppLinkGrant(t *testing.T) {
	tests := []struct {
		name    string
		grant   *AppLinkGrant
		scope   model.AccountAuthLocalApiScope
		wantErr string
	}{
		{
			name:  "nil grant is valid on any scope",
			grant: nil,
			scope: model.AccountAuth_Limited,
		},
		{
			name:  "valid read grant",
			grant: &AppLinkGrant{Version: 1, Spaces: []string{"s1"}, Perms: AppLinkPermsRead},
			scope: model.AccountAuth_JsonAPI,
		},
		{
			name:  "valid readwrite grant",
			grant: &AppLinkGrant{Version: 1, Spaces: []string{"s1", "s2"}, Perms: AppLinkPermsReadWrite},
			scope: model.AccountAuth_JsonAPI,
		},
		{
			name:    "grant on Limited scope",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{"s1"}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_Limited,
			wantErr: "grant requires JsonAPI scope",
		},
		{
			name:    "grant on Full scope",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{"s1"}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_Full,
			wantErr: "grant requires JsonAPI scope",
		},
		{
			name:    "nil spaces",
			grant:   &AppLinkGrant{Version: 1, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "spaces must be non-empty",
		},
		{
			name:    "empty spaces",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "spaces must be non-empty",
		},
		{
			name:    "empty space id",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{"s1", ""}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "empty space id",
		},
		{
			name:    "unknown perms",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{"s1"}, Perms: "admin"},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: `unknown perms "admin"`,
		},
		{
			name:    "empty perms",
			grant:   &AppLinkGrant{Version: 1, Spaces: []string{"s1"}, Perms: ""},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "unknown perms",
		},
		{
			name:    "unknown version",
			grant:   &AppLinkGrant{Version: 2, Spaces: []string{"s1"}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "unknown grant version 2",
		},
		{
			name:    "zero version",
			grant:   &AppLinkGrant{Version: 0, Spaces: []string{"s1"}, Perms: AppLinkPermsRead},
			scope:   model.AccountAuth_JsonAPI,
			wantErr: "unknown grant version 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// when
			err := ValidateAppLinkGrant(tt.grant, tt.scope)

			// then
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidGrant)
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}

	t.Run("generate rejects an invalid grant before writing", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)

		// when
		_, err = generate(dir, pk, "no-spaces", model.AccountAuth_JsonAPI, 0, &AppLinkGrant{Version: 1, Perms: AppLinkPermsRead})

		// then
		require.ErrorIs(t, err, ErrInvalidGrant)
		files, globErr := filepath.Glob(filepath.Join(dir, "*.json"))
		require.NoError(t, globErr)
		require.Empty(t, files, "nothing may be written for a rejected grant")
	})
}

func TestUpdateAppLinkGrant(t *testing.T) {
	newTestWallet := func(t *testing.T) (*wallet, string) {
		t.Helper()
		tmp := t.TempDir()
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		return &wallet{repoPath: tmp, accountKey: pk}, filepath.Join(tmp, appLinkKeysDirectory)
	}

	t.Run("grant attaches in place to a legacy-format json api key", func(t *testing.T) {
		// given: THE in-place upgrade path — an old raw-base64 key gets scoped
		// without the holder ever learning a new secret
		w, dir := newTestWallet(t)
		appKey, err := writeLegacyV1AppLink(dir, w.accountKey, &AppLinkInfo{
			AppName: "legacy-json-api",
			Scope:   int(model.AccountAuth_JsonAPI),
		})
		require.NoError(t, err)
		appHash := fmt.Sprintf("%x", sha256.Sum256(mustDecodeBase64(t, appKey)))
		want := testGrant()

		// when
		err = w.UpdateAppLinkGrant(appHash, want)

		// then: the SAME old key string now resolves to a granted record in a
		// ver-2 envelope
		require.NoError(t, err)
		got, err := w.ReadAppLink(appKey)
		require.NoError(t, err)
		assert.Equal(t, want, got.Grant)
		assert.Equal(t, "legacy-json-api", got.AppName)
		assert.Equal(t, ver2, envelopeVersionOnDisk(t, dir, appHash))
		// and a pre-grant binary refuses the upgraded file rather than
		// serving it unscoped
		require.ErrorContains(t, readV1OnlyEnvelopeVersion(t, dir, appHash), "unsupported version 2")
	})

	t.Run("replacing a grant", func(t *testing.T) {
		// given
		w, _ := newTestWallet(t)
		first := &AppLinkGrant{Version: 1, Spaces: []string{"space1"}, Perms: AppLinkPermsRead}
		info, err := w.PersistAppLink("edited", model.AccountAuth_JsonAPI, 0, first)
		require.NoError(t, err)
		want := &AppLinkGrant{Version: 1, Spaces: []string{"space2", "space3"}, Perms: AppLinkPermsReadWrite}

		// when
		err = w.UpdateAppLinkGrant(info.AppHash, want)

		// then
		require.NoError(t, err)
		got, err := w.ReadAppLink(info.AppKey)
		require.NoError(t, err)
		assert.Equal(t, want, got.Grant)
	})

	t.Run("clearing a grant returns the envelope to ver 1", func(t *testing.T) {
		// given
		w, dir := newTestWallet(t)
		info, err := w.PersistAppLink("cleared", model.AccountAuth_JsonAPI, 0, testGrant())
		require.NoError(t, err)
		require.Equal(t, ver2, envelopeVersionOnDisk(t, dir, info.AppHash))

		// when
		err = w.UpdateAppLinkGrant(info.AppHash, nil)

		// then
		require.NoError(t, err)
		got, err := w.ReadAppLink(info.AppKey)
		require.NoError(t, err)
		assert.Nil(t, got.Grant)
		assert.Equal(t, ver1, envelopeVersionOnDisk(t, dir, info.AppHash))
	})

	t.Run("unknown hash", func(t *testing.T) {
		// given
		w, _ := newTestWallet(t)
		_, err := w.PersistAppLink("other", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		// when
		err = w.UpdateAppLinkGrant("no-such-hash", testGrant())

		// then
		require.ErrorIs(t, err, ErrAppLinkNotFound)
	})

	t.Run("grant on a Limited key is refused", func(t *testing.T) {
		// given
		w, _ := newTestWallet(t)
		info, err := w.PersistAppLink("clipper", model.AccountAuth_Limited, 0, nil)
		require.NoError(t, err)

		// when
		err = w.UpdateAppLinkGrant(info.AppHash, testGrant())

		// then: still readable and still unscoped
		require.ErrorIs(t, err, ErrInvalidGrant)
		got, readErr := w.ReadAppLink(info.AppKey)
		require.NoError(t, readErr)
		assert.Nil(t, got.Grant)
	})

	t.Run("invalid grant leaves the file untouched", func(t *testing.T) {
		// given
		w, _ := newTestWallet(t)
		want := testGrant()
		info, err := w.PersistAppLink("kept", model.AccountAuth_JsonAPI, 0, want)
		require.NoError(t, err)

		// when
		err = w.UpdateAppLinkGrant(info.AppHash, &AppLinkGrant{Version: 1, Spaces: []string{"s"}, Perms: "admin"})

		// then
		require.ErrorIs(t, err, ErrInvalidGrant)
		got, readErr := w.ReadAppLink(info.AppKey)
		require.NoError(t, readErr)
		assert.Equal(t, want, got.Grant)
	})

	t.Run("v0 envelopes cannot be updated", func(t *testing.T) {
		// given: a v0 payload is encrypted with the app key itself, which the
		// hash-addressed update path does not have
		w, dir := newTestWallet(t)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		appKey, err := writeAppLinkFileV0(w.accountKey, dir, &AppLinkInfo{
			AppName: "ancient",
			Scope:   int(model.AccountAuth_JsonAPI),
		})
		require.NoError(t, err)
		appHash := fmt.Sprintf("%x", sha256.Sum256(mustDecodeBase64(t, appKey)))

		// when
		err = w.UpdateAppLinkGrant(appHash, testGrant())

		// then
		require.ErrorContains(t, err, "does not support grant updates")
	})
}

func mustDecodeBase64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	return raw
}

// TestUpdateRacingRevoke pins that a revoked key stays revoked when
// UpdateAppLinkGrant runs concurrently: without the wallet's appLinkMu, the
// update could open the file, lose the race to revoke's os.Remove, and then
// rename its rewrite back into place — resurrecting a fully valid record for
// a key the user was told is gone. Whatever the interleaving, the postcondition
// is the same: the file is gone and the key no longer authenticates.
func TestUpdateRacingRevoke(t *testing.T) {
	for i := 0; i < 25; i++ {
		tmp := t.TempDir()
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		w := &wallet{repoPath: tmp, accountKey: pk}
		info, err := w.PersistAppLink("raced", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)

		var wg sync.WaitGroup
		var updateErr, revokeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			updateErr = w.UpdateAppLinkGrant(info.AppHash, testGrant())
		}()
		go func() {
			defer wg.Done()
			revokeErr = w.RevokeAppLink(info.AppHash)
		}()
		wg.Wait()

		// the file existed, so the revoke must have succeeded; the update
		// either ran first (nil) or found the file already gone
		require.NoError(t, revokeErr)
		if updateErr != nil {
			require.ErrorIs(t, updateErr, ErrAppLinkNotFound)
		}
		_, statErr := os.Stat(filepath.Join(tmp, appLinkKeysDirectory, info.AppHash+".json"))
		require.True(t, os.IsNotExist(statErr), "revoked app link file must stay gone, update won an illegal race")
		_, readErr := w.ReadAppLink(info.AppKey)
		require.ErrorIs(t, readErr, ErrAppLinkNotFound)
	}
}

func TestListVerification(t *testing.T) {
	t.Run("a ver-1 envelope carrying a grant lists hash-only", func(t *testing.T) {
		// given: the same file load refuses — list must not advertise the
		// grant the authentication path rejects
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		appKey := base64.StdEncoding.EncodeToString(key.Bytes())
		file, err := buildEnvelope(ver1, key.Bytes(), pk, &AppLinkInfo{
			AppKey:  appKey,
			AppName: "smuggled",
			Scope:   int(model.AccountAuth_JsonAPI),
			Grant:   testGrant(),
		})
		require.NoError(t, err)
		raw, err := json.Marshal(&file)
		require.NoError(t, err)
		appHash := fmt.Sprintf("%x", sha256.Sum256(key.Bytes()))
		require.NoError(t, os.WriteFile(filepath.Join(dir, appHash+".json"), raw, 0o600))

		// when
		entries, err := list(dir, pk)

		// then: still listed (revocable), but degraded — no grant, no name
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, appHash, entries[0].AppHash)
		assert.Nil(t, entries[0].Grant)
		assert.Empty(t, entries[0].AppName)
	})

	t.Run("a tampered signature lists hash-only", func(t *testing.T) {
		// given
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		info, err := generate(dir, pk, "tampered", model.AccountAuth_JsonAPI, 0, testGrant())
		require.NoError(t, err)
		path := filepath.Join(dir, info.AppHash+".json")
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var file fileV1
		require.NoError(t, json.Unmarshal(raw, &file))
		file.Signature[0] ^= 0xff
		raw, err = json.Marshal(&file)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, raw, 0o600))

		// when
		entries, err := list(dir, pk)

		// then
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, info.AppHash, entries[0].AppHash)
		assert.Nil(t, entries[0].Grant)
		assert.Empty(t, entries[0].AppName)
	})

	t.Run("a file copied under another hash lists hash-only", func(t *testing.T) {
		// given: a valid record wholesale-copied to a different filename — the
		// filename↔key binding must refuse it just as updateGrant does
		tmp := t.TempDir()
		dir := filepath.Join(tmp, appLinkKeysDirectory)
		pk, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		info, err := generate(dir, pk, "copied", model.AccountAuth_JsonAPI, 0, nil)
		require.NoError(t, err)
		raw, err := os.ReadFile(filepath.Join(dir, info.AppHash+".json"))
		require.NoError(t, err)
		otherHash := strings.Repeat("ab", 32)
		require.NoError(t, os.WriteFile(filepath.Join(dir, otherHash+".json"), raw, 0o600))

		// when
		entries, err := list(dir, pk)

		// then: the copy is degraded, the original untouched
		require.NoError(t, err)
		require.Len(t, entries, 2)
		byHash := map[string]*AppLinkInfo{}
		for _, e := range entries {
			byHash[e.AppHash] = e
		}
		require.Empty(t, byHash[otherHash].AppName)
		require.Equal(t, "copied", byHash[info.AppHash].AppName)
	})
}

func TestAppLinkGrantFromProto_UnknownPerm(t *testing.T) {
	// given: gogo unmarshals unknown enum values verbatim, so a client really
	// can send perm=99; it must map to a perms value that validation REJECTS —
	// an unrecognized permission must never widen into a recognized one
	grant := AppLinkGrantFromProto(&model.AccountAuthAppGrant{
		SpaceIds: []string{"s1"},
		Perm:     model.AccountAuthAppGrantPerm(99),
	})

	// when
	err := ValidateAppLinkGrant(grant, model.AccountAuth_JsonAPI)

	// then
	require.ErrorIs(t, err, ErrInvalidGrant)
	require.ErrorContains(t, err, "unknown perms")
}
