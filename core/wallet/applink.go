package wallet

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	json "encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/gcm"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	appLinkKeysDirectory = "auth"
	ver1                 = 1
	// ver2 marks an envelope whose sealed payload carries a grant. The layout
	// is identical to ver 1 — the bump exists so a binary that only knows
	// ver 1 REFUSES the file (its load hard-errors on unknown versions)
	// instead of silently honoring a scoped key as unscoped. Unscoped keys
	// keep writing ver 1 so a downgrade never locks them out.
	ver2 = 2
)

const (
	// AppLinkPermsRead and AppLinkPermsReadWrite are the only valid Perms
	// values of an AppLinkGrant.
	AppLinkPermsRead      = "read"
	AppLinkPermsReadWrite = "readwrite"
	// appLinkGrantVersion is the only grant schema version this binary writes
	// or accepts. Any new grant dimension that NARROWS the key (Types is the
	// planned one) MUST bump this version: an older binary drops unknown JSON
	// fields, so under the same version it would enforce the grant WIDER than
	// written — the exact downgrade widening the version gate exists to
	// prevent (ValidateAppLinkGrant hard-rejects unknown versions, fail
	// closed). Only fields that cannot widen the effective permission may
	// ride the same version.
	appLinkGrantVersion = 1
)

// New-format app key: <prefix>_<body>_<checksum>, following the GitHub 2021
// token redesign. The fixed prefix makes leaked keys greppable by secret
// scanners and recognizable to humans as Anytype JSON-API keys.
//
// The body is the 32 raw key bytes in lowercase unpadded base32 (52 chars of
// [a-z2-7]). The alphabet is alphanumeric-only by hard requirement (spec P1
// §1b): `+`, `/` and `=` would break \b-anchored scanner rules mid-token, be
// mangled by form-encoding, and need escaping in the shell/JSON config files
// these keys live in. It also keeps the key a valid RFC 6750 token68 and the
// `_` separators unambiguous.
//
// The checksum is CRC32-IEEE over `prefix + "_" + body` — the prefix is
// covered so a mangled prefix is caught too — rendered as 8 lowercase hex
// chars, so a mistyped key is rejected before any disk access. It is a typo
// detector, NOT an authenticator (CRC over public bytes is trivially
// forgeable): it must never gate or select an authorization path.
//
// The published pattern for scanner rules is a length RANGE, never a fixed
// length (a future body encoding must not silently defeat third-party rules):
//
//	\banytype_[0-9A-Za-z]{40,60}_[0-9a-f]{8}\b
//
// Format changes version by minting a NEW prefix namespace, never an inline
// version field; the read path accepts every historical format forever.
const (
	appKeyPrefix            = "anytype"
	appKeySeparator         = "_"
	appKeyChecksumHexLength = 8
)

// appKeyBodyEncoding renders the raw key bytes for the prefixed format:
// lowercase unpadded base32, the stdlib option the spec sanctions for an
// alphanumeric-only body.
var appKeyBodyEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

var (
	ErrAppLinkNotFound = fmt.Errorf("app link file not found in the account directory")
	// ErrAppLinkExpired is returned by ReadAppLink when the link's ExpireAt
	// has passed. The file stays on disk (ListApps still shows it) but the key
	// no longer authenticates.
	ErrAppLinkExpired = fmt.Errorf("app link expired")
	// ErrInvalidGrant is wrapped by every grant validation failure, so callers
	// can map any of them to a single bad-input code.
	ErrInvalidGrant = fmt.Errorf("invalid app link grant")
)

type AppLinkInfo struct {
	AppHash   string `json:"-"` // filled at read time
	AppKey    string `json:"app_key"`
	AppName   string `json:"app_name"`
	CreatedAt int64  `json:"created_at"`
	ExpireAt  int64  `json:"expire_at"`
	Scope     int    `json:"scope"`
	// Grant scopes the key to a set of spaces with a permission level. It
	// lives inside the sealed, HMAC-bound, signed payload, so nothing short
	// of the account key can forge or widen it on disk. Grant == nil means a
	// legacy unscoped key; enforcement keys off the grant, never off the key
	// string format.
	Grant *AppLinkGrant `json:"grant,omitempty"`
}

// AppLinkGrant is the scope record of a JSON-API key: which spaces the key
// may touch and with which permission. Grants only narrow — a key with a
// grant is never allowed more than an unscoped key.
type AppLinkGrant struct {
	// Version is the GRANT SCHEMA version — unrelated to the envelope's
	// file-format version (fileV1.Version, the `ver` a granted key writes as
	// 2). appLinkGrantVersion is the only value this binary writes or
	// accepts; bumping it makes older binaries refuse the key.
	Version int      `json:"v"`
	Spaces  []string `json:"spaces"` // space ids; must be non-empty
	Perms   string   `json:"perms"`  // AppLinkPermsRead | AppLinkPermsReadWrite
	// P2 reserves: Types map[string][]string — spaceId → ot-… uniqueKeys
	// (a narrowing dimension: adding it must bump the version, see
	// appLinkGrantVersion)
}

// ValidateAppLinkGrant is the single persist-time (and read-time, defense in
// depth) gate for grants. A nil grant is always valid — it is the legacy
// unscoped key. A non-nil grant is valid only on JsonAPI-scope keys: Full is
// a separate kind, never a modifier inside scoped semantics, and Limited is a
// gRPC/clipper credential that never reaches the JSON API.
func ValidateAppLinkGrant(grant *AppLinkGrant, scope model.AccountAuthLocalApiScope) error {
	if grant == nil {
		return nil
	}
	if scope != model.AccountAuth_JsonAPI {
		return fmt.Errorf("%w: grant requires JsonAPI scope, got %s", ErrInvalidGrant, scope.String())
	}
	if grant.Version != appLinkGrantVersion {
		return fmt.Errorf("%w: unknown grant version %d", ErrInvalidGrant, grant.Version)
	}
	if len(grant.Spaces) == 0 {
		return fmt.Errorf("%w: spaces must be non-empty", ErrInvalidGrant)
	}
	for _, spaceId := range grant.Spaces {
		if spaceId == "" {
			return fmt.Errorf("%w: empty space id", ErrInvalidGrant)
		}
	}
	switch grant.Perms {
	case AppLinkPermsRead, AppLinkPermsReadWrite:
	default:
		return fmt.Errorf("%w: unknown perms %q", ErrInvalidGrant, grant.Perms)
	}
	return nil
}

// Proto converts the grant for the gRPC surface (ListApps, WalletCreateSession).
func (g *AppLinkGrant) Proto() *model.AccountAuthAppGrant {
	if g == nil {
		return nil
	}
	perm := model.AccountAuthAppGrant_Read
	if g.Perms == AppLinkPermsReadWrite {
		perm = model.AccountAuthAppGrant_ReadWrite
	}
	return &model.AccountAuthAppGrant{
		SpaceIds: g.Spaces,
		Perm:     perm,
	}
}

// AppLinkGrantFromProto builds the persistable grant from its proto form. An
// unknown enum value maps to an empty Perms string, which validation rejects
// — an unrecognized permission must never widen into one that is recognized.
func AppLinkGrantFromProto(grant *model.AccountAuthAppGrant) *AppLinkGrant {
	if grant == nil {
		return nil
	}
	var perms string
	switch grant.Perm {
	case model.AccountAuthAppGrant_Read:
		perms = AppLinkPermsRead
	case model.AccountAuthAppGrant_ReadWrite:
		perms = AppLinkPermsReadWrite
	}
	return &AppLinkGrant{
		Version: appLinkGrantVersion,
		Spaces:  grant.SpaceIds,
		Perms:   perms,
	}
}

func (r *wallet) ReadAppLink(appKey string) (*AppLinkInfo, error) {
	if r.repoPath == "" {
		return nil, fmt.Errorf("repo path is not set")
	}
	if r.accountKey == nil {
		return nil, fmt.Errorf("account is not set")
	}

	r.appLinkMu.RLock()
	defer r.appLinkMu.RUnlock()
	info, err := load(filepath.Join(r.repoPath, appLinkKeysDirectory), appKey, r.accountKey)
	if err != nil {
		return nil, fmt.Errorf("load app link: %w", err)
	}
	if info.ExpireAt > 0 && time.Now().Unix() > info.ExpireAt {
		return nil, ErrAppLinkExpired
	}
	return info, nil
}

// PersistAppLink creates a new app link. expireAt is a unix timestamp after
// which ReadAppLink refuses the key; 0 means the key never expires. Only the
// explicit CreateApp path sets a non-zero value — the challenge pairing path
// keeps 0 until the consent picker exists. A non-nil grant scopes the key and
// is valid only with JsonAPI scope.
func (r *wallet) PersistAppLink(name string, scope model.AccountAuthLocalApiScope, expireAt int64, grant *AppLinkGrant) (app *AppLinkInfo, err error) {
	r.appLinkMu.Lock()
	defer r.appLinkMu.Unlock()
	return generate(filepath.Join(r.repoPath, appLinkKeysDirectory), r.accountKey, name, scope, expireAt, grant)
}

// UpdateAppLinkGrant replaces the grant of an existing app link in place: the
// key string the holder uses is untouched, only the sealed record changes.
// A nil grant clears the scoping (the widen-requires-re-consent contract is
// the caller's — the desktop UI's — not the wallet's). Works on any JsonAPI
// key, legacy key format included: this is the in-place upgrade path that
// scopes a key without redistributing a secret.
func (r *wallet) UpdateAppLinkGrant(appHash string, grant *AppLinkGrant) error {
	if r.repoPath == "" {
		return fmt.Errorf("repo path is not set")
	}
	if r.accountKey == nil {
		return fmt.Errorf("account is not set")
	}
	r.appLinkMu.Lock()
	defer r.appLinkMu.Unlock()
	return updateGrant(filepath.Join(r.repoPath, appLinkKeysDirectory), appHash, grant, r.accountKey)
}

// ListAppLinks returns a list of all app links for this repo directory
func (r *wallet) ListAppLinks() ([]*AppLinkInfo, error) {
	if r.repoPath == "" {
		return nil, fmt.Errorf("repo path is not set")
	}
	if r.accountKey == nil {
		return nil, fmt.Errorf("account is not set")
	}

	r.appLinkMu.RLock()
	defer r.appLinkMu.RUnlock()
	return list(filepath.Join(r.repoPath, appLinkKeysDirectory), r.accountKey)
}

// RevokeAppLink removes an app link based on its app hash.
func (r *wallet) RevokeAppLink(appHash string) error {
	if r.repoPath == "" {
		return fmt.Errorf("repo path is not set")
	}

	r.appLinkMu.Lock()
	defer r.appLinkMu.Unlock()
	return revoke(filepath.Join(r.repoPath, appLinkKeysDirectory), appHash)
}

func generate(dir string, accountPriv crypto.PrivKey, appName string, scope model.AccountAuthLocalApiScope, expireAt int64, grant *AppLinkGrant) (info *AppLinkInfo, _ error) {
	if err := ValidateAppLinkGrant(grant, scope); err != nil {
		return nil, fmt.Errorf("validate grant: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	key, err := crypto.NewRandomAES()
	if err != nil {
		return nil, err
	}
	// From the format flip, JSON-API keys mint ONLY in the prefixed +
	// checksummed format. Limited keys keep the raw base64 format: they are a
	// gRPC/clipper credential, not a JSON-API key, and the prefix would
	// falsely brand them as one. The format is provenance and scanner signal
	// only — no authorization decision may ever branch on it.
	appKey := base64.StdEncoding.EncodeToString(key.Bytes())
	if scope == model.AccountAuth_JsonAPI {
		appKey = formatAppKey(key.Bytes())
	}
	info = &AppLinkInfo{
		AppHash:   fmt.Sprintf("%x", sha256.Sum256(key.Bytes())),
		AppKey:    appKey,
		AppName:   appName,
		CreatedAt: time.Now().Unix(),
		ExpireAt:  expireAt,
		Scope:     int(scope),
		Grant:     grant,
	}
	file, err := buildEnvelope(envelopeVersionFor(grant), key.Bytes(), accountPriv, info)
	if err != nil {
		return nil, fmt.Errorf("build app link envelope: %w", err)
	}
	name := fmt.Sprintf("%s.json", info.AppHash)
	fp, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	return info, json.NewEncoder(fp).Encode(&file)
}

// envelopeVersionFor: a grant rides only in a ver-2 envelope so an older
// binary refuses the file outright (downgrade fail-closed); unscoped keys
// keep ver 1 so a downgrade never locks them out.
func envelopeVersionFor(grant *AppLinkGrant) int {
	if grant != nil {
		return ver2
	}
	return ver1
}

// formatAppKey renders raw key bytes in the new prefixed+checksummed format;
// see the appKeyPrefix comment for the layout and its rationale.
func formatAppKey(raw []byte) string {
	body := appKeyBodyEncoding.EncodeToString(raw)
	return appKeyPrefix + appKeySeparator + body + appKeySeparator + appKeyChecksum(body)
}

// appKeyChecksum covers prefix, separator and body so a mangled prefix is
// caught alongside a mistyped body.
func appKeyChecksum(body string) string {
	return fmt.Sprintf("%0*x", appKeyChecksumHexLength, crc32.ChecksumIEEE([]byte(appKeyPrefix+appKeySeparator+body)))
}

// appKeyBytes recovers the raw key bytes from either key format. New-format
// keys have their checksum verified here, so a typo is rejected before any
// disk access; both formats decode to the same 32 raw bytes, whose sha256
// derives the on-disk filename — so no existing file lookup changes. Legacy
// keys are plain std base64 and can never contain the underscore separator,
// so the prefix check cannot misfire on them.
func appKeyBytes(appKey string) ([]byte, error) {
	if body, found := strings.CutPrefix(appKey, appKeyPrefix+appKeySeparator); found {
		sep := strings.LastIndex(body, appKeySeparator)
		if sep < 0 {
			return nil, fmt.Errorf("malformed app key: missing checksum")
		}
		body, checksum := body[:sep], body[sep+1:]
		if checksum != appKeyChecksum(body) {
			return nil, fmt.Errorf("app key checksum mismatch")
		}
		key, err := appKeyBodyEncoding.DecodeString(body)
		if err != nil {
			return nil, fmt.Errorf("decode app key body: %w", err)
		}
		return key, nil
	}
	key, err := base64.StdEncoding.DecodeString(appKey)
	if err != nil {
		return nil, fmt.Errorf("decode app key: %w", err)
	}
	return key, nil
}

// load and verify the app link file. The single parse point for both key
// formats: appKeyBytes rejects a mistyped new-format key before any disk
// access, and every format resolves to the same raw-bytes-derived filename.
func load(dir, appKey string, accountPriv crypto.PrivKey) (*AppLinkInfo, error) {
	key, err := appKeyBytes(appKey)
	if err != nil {
		return nil, fmt.Errorf("parse app key: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256(key)))
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrAppLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read app link file: %w", err)
	}

	// sniff version
	var peek struct {
		Version int `json:"ver"`
	}
	if err = json.Unmarshal(raw, &peek); err != nil {
		return nil, fmt.Errorf("decode app link file: %w", err)
	}

	var info *AppLinkInfo
	switch peek.Version {
	case 0 /* field missing */ :
		var v0 fileV0
		if err = json.Unmarshal(raw, &v0); err != nil {
			return nil, fmt.Errorf("decode app link file: %w", err)
		}
		info, err = verifyAndOpenV0(key, accountPriv, &v0)

	case ver1, ver2:
		// same layout; ver 2 only marks that the sealed payload carries a
		// grant, so binaries that predate grants refuse it (fail closed)
		var v1 fileV1
		if err = json.Unmarshal(raw, &v1); err != nil {
			return nil, fmt.Errorf("decode app link file: %w", err)
		}
		info, err = verifyAndOpenV1(key, accountPriv, &v1)

	default:
		return nil, fmt.Errorf("unsupported version %d", peek.Version)
	}
	if err != nil {
		return nil, fmt.Errorf("verify app link envelope: %w", err)
	}
	if err = checkGrantEnvelope(info, peek.Version); err != nil {
		return nil, err
	}
	// AppHash is json:"-" (never stored inside the sealed payload), so it must
	// be re-derived here. Session revocation keys off this value: if it were
	// left empty, every session minted from the key would be tracked under ""
	// and LinkLocalRevokeApp could never find it.
	info.AppHash = fmt.Sprintf("%x", sha256.Sum256(key))
	return info, nil
}

// checkGrantEnvelope holds the read-time grant gates shared by load and list:
// a grant must ride a ver-2 envelope — in a lower version an older binary
// would read the file and silently drop the grant — and, defense in depth, a
// stored grant this binary cannot validate must not be honored (the payload
// is signed, so an invalid one can only come from a writer with different
// validation rules).
func checkGrantEnvelope(info *AppLinkInfo, envelopeVersion int) error {
	if info.Grant == nil {
		return nil
	}
	if envelopeVersion < ver2 {
		return fmt.Errorf("app link envelope version %d must not carry a grant", envelopeVersion)
	}
	if err := ValidateAppLinkGrant(info.Grant, model.AccountAuthLocalApiScope(info.Scope)); err != nil { // nolint:gosec
		return fmt.Errorf("stored grant: %w", err)
	}
	return nil
}

// updateGrant rewrites the app link file named by appHash with the given
// grant (nil clears it), leaving every other field — most importantly the key
// string itself — unchanged. The envelope version follows the grant: ver 2
// with one, ver 1 without, so the downgrade fail-closed property holds for
// keys scoped after issuance too. v0 envelopes cannot be updated: their
// payload is encrypted with the app key itself, which this path does not
// have (it works from the hash alone).
func updateGrant(dir, appHash string, grant *AppLinkGrant, accountPriv crypto.PrivKey) error {
	path := filepath.Join(dir, appHash+".json")
	// The whole file is read into memory up front — no handle on path may be
	// live when os.Rename replaces it below (Windows opens files without
	// delete sharing, so a rename over an open file fails there).
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrAppLinkNotFound
	}
	if err != nil {
		return fmt.Errorf("read app link file: %w", err)
	}

	var peek struct {
		Version int `json:"ver"`
	}
	if err = json.Unmarshal(raw, &peek); err != nil {
		return fmt.Errorf("decode app link file: %w", err)
	}

	switch peek.Version {
	case ver1, ver2:
	default:
		return fmt.Errorf("app link envelope version %d does not support grant updates", peek.Version)
	}
	// The sealed payload carries the key string, so the whole envelope is
	// re-verified before the rewrite — signature, HMAC, and the filename↔key
	// binding (the HMAC only proves key↔record consistency, which a
	// wholesale-copied file also passes; the filename check pins the record
	// to the hash the caller named, so a swapped file cannot be re-signed
	// under another name).
	info, err := openVerifiedV1(raw, appHash, peek.Version, accountPriv)
	if err != nil {
		return fmt.Errorf("verify app link file: %w", err)
	}
	key, err := appKeyBytes(info.AppKey)
	if err != nil {
		return fmt.Errorf("recover app key bytes: %w", err)
	}
	if err = ValidateAppLinkGrant(grant, model.AccountAuthLocalApiScope(info.Scope)); err != nil { // nolint:gosec
		return fmt.Errorf("validate grant: %w", err)
	}
	info.Grant = grant
	updated, err := buildEnvelope(envelopeVersionFor(grant), key, accountPriv, info)
	if err != nil {
		return fmt.Errorf("build app link envelope: %w", err)
	}
	// Write-fsync-rename: the replacement is atomic, so a reader sees either
	// the old record or the new one, never a truncated in-place write, and
	// the fsync makes the new bytes durable before the rename can commit them.
	// (The directory entry itself is not fsynced — a crash in that window can
	// revert to the OLD record, which is a valid state, not a bricked key.)
	tmp, err := os.CreateTemp(dir, appHash+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp app link file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err = json.NewEncoder(tmp).Encode(&updated); err != nil {
		tmp.Close()
		return fmt.Errorf("encode app link file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp app link file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp app link file: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace app link file: %w", err)
	}
	return nil
}

// List reads all app link files in the directory.
// For v0 files, only the AppHash field will be populated.
// For v1/v2 files, it includes the whole AppLinkInfo — but only after the
// same signature/HMAC/filename/grant verification the authentication path
// runs: an entry that verification refuses degrades to AppHash-only (it stays
// listable so the user can still see and revoke it), so ListApps can never
// advertise a name or a grant that ReadAppLink would reject.
func list(dir string, accountPriv crypto.PrivKey) ([]*AppLinkInfo, error) {
	// Ensure directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil // Return empty slice if directory doesn't exist
	}

	// Read all .json files in the directory
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob app link files: %w", err)
	}

	var result []*AppLinkInfo
	for _, path := range files {
		// Extract app hash from filename
		appHash := strings.TrimSuffix(filepath.Base(path), ".json")

		raw, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files we can't read
		}
		var peek struct {
			Version int `json:"ver"`
		}
		if err = json.Unmarshal(raw, &peek); err != nil {
			continue // Skip malformed files
		}

		info := &AppLinkInfo{
			AppHash: appHash,
		}
		if (peek.Version == ver1 || peek.Version == ver2) && accountPriv != nil {
			if verified, err := openVerifiedV1(raw, appHash, peek.Version, accountPriv); err == nil {
				info = verified
			}
		}
		result = append(result, info)
	}

	return result, nil
}

// openVerifiedV1 decodes a v1/v2 app link file and runs the full read-path
// verification without possessing the key string up front: the sealed payload
// carries the key, so after decryption the envelope can be re-verified —
// signature, HMAC, filename↔key binding and the grant gates — exactly as load
// does. list uses it so display and authentication can never disagree.
func openVerifiedV1(raw []byte, appHash string, envelopeVersion int, accountPriv crypto.PrivKey) (*AppLinkInfo, error) {
	var v1 fileV1
	if err := json.Unmarshal(raw, &v1); err != nil {
		return nil, fmt.Errorf("decode app link file: %w", err)
	}
	plain, err := accountPriv.Decrypt(v1.Info)
	if err != nil {
		return nil, fmt.Errorf("decrypt app link payload: %w", err)
	}
	var sealed AppLinkInfo
	if err = json.Unmarshal(plain, &sealed); err != nil {
		return nil, fmt.Errorf("unmarshal app link payload: %w", err)
	}
	key, err := appKeyBytes(sealed.AppKey)
	if err != nil {
		return nil, fmt.Errorf("recover app key bytes: %w", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(key)) != appHash {
		return nil, fmt.Errorf("app link file does not match its hash")
	}
	info, err := verifyAndOpenV1(key, accountPriv, &v1)
	if err != nil {
		return nil, fmt.Errorf("verify app link envelope: %w", err)
	}
	if err = checkGrantEnvelope(info, envelopeVersion); err != nil {
		return nil, fmt.Errorf("check grant envelope: %w", err)
	}
	info.AppHash = appHash
	return info, nil
}

// Revoke removes an app link file based on its app hash.
// Returns an error if the file doesn't exist.
func revoke(dir, appHash string) error {
	filePath := filepath.Join(dir, appHash+".json")

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrAppLinkNotFound
	}

	// Delete the file
	return os.Remove(filePath)
}

type fileV0 struct {
	Payload   []byte `json:"payload"`   // AES-GCM(appKey, AppLinkInfo)
	Signature []byte `json:"signature"` // Ed25519(accountPriv, payload)
}

func verifyAndOpenV0(appKey []byte, accountPriv crypto.PrivKey, f *fileV0) (*AppLinkInfo, error) {
	ok, _ := accountPriv.GetPublic().Verify(f.Payload, f.Signature)
	if !ok {
		return nil, errors.New("v0 signature invalid")
	}

	key, err := symmetric.FromBytes(appKey)
	if err != nil {
		return nil, err
	}

	r, err := gcm.New(key).DecryptReader(bytes.NewReader(f.Payload))
	if err != nil {
		return nil, err
	}

	var info AppLinkInfo
	d := json.NewDecoder(r)
	if err = d.Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

// fileV1 is the JSON-encoded on-disk structure introduced in format-version 1
// and shared unchanged by format-version 2.
type fileV1 struct {
	// Version is the *file-format* version tag: 1 for unscoped keys, 2 when
	// the sealed payload carries a grant. The layouts are identical — ver 2
	// exists so a binary that predates grants refuses the file instead of
	// silently reading the key as unscoped. (Future layout changes should
	// bump this value and add a new struct.)
	Version int `json:"ver"`

	// Info contains the envelope-encrypted AppLinkInfo:
	//   X25519-SealedBox (accountPub, plaintextJSON(AppLinkInfo)).
	// Only the user's account private key can open it, so the payload stays
	// confidential even if the file is copied.
	Info []byte `json:"info"`

	// Auth is an integrity MAC:
	//   HMAC-SHA-256(appKey,  ver || info)
	// It proves that the same per-app symmetric key that named the file
	// was also present when the record was created (prevents file swapping).
	Auth []byte `json:"auth"`

	// Signature is the wallet owner's attestation:
	//   Ed25519(accountPriv, ver || info || auth)
	// It cryptographically binds the record to the specific wallet account
	// and prevents any on-disk modification or replay from another account.
	Signature []byte `json:"sig"`
}

// buildEnvelope writes the json-ready struct for envelope versions 1 and 2
// (same layout; the version rides in the signed bytes, so a downgrade of the
// tag on disk breaks the signature).
func buildEnvelope(ver int, appKey []byte, accountPriv crypto.PrivKey, info *AppLinkInfo) (fileV1, error) {
	msg, err := json.Marshal(info)
	if err != nil {
		return fileV1{}, err
	}

	// 1. encrypt Info with X25519 sealed-box
	sealed, err := accountPriv.GetPublic().Encrypt(msg)
	if err != nil {
		return fileV1{}, err
	}

	// 2. auth = HMAC(appKey, ver||info)
	auth := hmacAuth(appKey, ver, sealed)

	// 3. signature = Ed25519(priv, ver||info||auth)
	sig, err := accountPriv.Sign(bytesForSig(ver, sealed, auth))
	if err != nil {
		return fileV1{}, err
	}
	return fileV1{
		Version:   ver,
		Info:      sealed,
		Auth:      auth,
		Signature: sig,
	}, nil
}

func verifyAndOpenV1(appKey []byte, accountPriv crypto.PrivKey, f *fileV1) (*AppLinkInfo, error) {
	// 1. verify Ed25519 signature
	if ok, _ := accountPriv.GetPublic().Verify(bytesForSig(f.Version, f.Info, f.Auth), f.Signature); !ok {
		return nil, errors.New("v1 ed25519 signature mismatch")
	}
	// 2. verify HMAC matches this appKey
	want := hmacAuth(appKey, f.Version, f.Info)
	if !hmac.Equal(want, f.Auth) {
		return nil, errors.New("v1 HMAC mismatch")
	}
	// 3. decrypt Info with X25519
	plain, err := accountPriv.Decrypt(f.Info)
	if err != nil {
		return nil, err
	}
	var info AppLinkInfo
	if err := json.Unmarshal(plain, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// hmacAuth = HMAC-SHA-256(appKey, ver||info)
func hmacAuth(appKey []byte, ver int, info []byte) []byte {
	mac := hmac.New(sha256.New, appKey)
	_ = binary.Write(mac, binary.BigEndian, ver)
	mac.Write(info)
	return mac.Sum(nil)
}

func bytesForSig(ver int, info, auth []byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(ver))
	buf.Write(info)
	buf.Write(auth)
	return buf.Bytes()
}
