package wallet

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	json "encoding/json"
	"errors"
	"fmt"
	"io"
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
)

var ErrAppLinkNotFound = fmt.Errorf("app link file not found in the account directory")

type AppLinkInfo struct {
	AppHash   string `json:"-"` // filled at read time
	AppKey    string `json:"app_key"`
	AppName   string `json:"app_name"`
	CreatedAt int64  `json:"created_at"`
	ExpireAt  int64  `json:"expire_at"`
	Scope     int    `json:"scope"`
}

func (r *wallet) ReadAppLink(appKey string) (*AppLinkInfo, error) {
	if r.repoPath == "" {
		return nil, fmt.Errorf("repo path is not set")
	}
	if r.accountKey == nil {
		return nil, fmt.Errorf("account is not set")
	}

	return load(filepath.Join(r.repoPath, appLinkKeysDirectory), appKey, r.accountKey)
}

func (r *wallet) PersistAppLink(name string, scope model.AccountAuthLocalApiScope) (app *AppLinkInfo, err error) {
	return generate(filepath.Join(r.repoPath, appLinkKeysDirectory), r.accountKey, name, scope)
}

// ListAppLinks returns a list of all app links for this repo directory
func (r *wallet) ListAppLinks() ([]*AppLinkInfo, error) {
	if r.repoPath == "" {
		return nil, fmt.Errorf("repo path is not set")
	}
	if r.accountKey == nil {
		return nil, fmt.Errorf("account is not set")
	}

	return list(filepath.Join(r.repoPath, appLinkKeysDirectory), r.accountKey)
}

// RevokeAppLink removes an app link based on its app hash.
func (r *wallet) RevokeAppLink(appHash string) error {
	if r.repoPath == "" {
		return fmt.Errorf("repo path is not set")
	}

	return revoke(filepath.Join(r.repoPath, appLinkKeysDirectory), appHash)
}

func generate(dir string, accountPriv crypto.PrivKey, appName string, scope model.AccountAuthLocalApiScope) (info *AppLinkInfo, _ error) {
	if err := os.MkdirAll(dir, 0o700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	key, err := crypto.NewRandomAES()
	if err != nil {
		return nil, err
	}
	appKey := base64.StdEncoding.EncodeToString(key.Bytes())
	info = &AppLinkInfo{
		AppHash:   fmt.Sprintf("%x", sha256.Sum256(key.Bytes())),
		AppKey:    appKey,
		AppName:   appName,
		CreatedAt: time.Now().Unix(),
		Scope:     int(scope),
	}
	file, err := buildV1(key.Bytes(), accountPriv, info)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s.json", info.AppHash)
	fp, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	return info, json.NewEncoder(fp).Encode(&file)
}

// load and verify the app link file
func load(dir, appKey string, accountPriv crypto.PrivKey) (*AppLinkInfo, error) {
	key, err := base64.StdEncoding.DecodeString(appKey)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.json", sha256.Sum256(key)))
	fp, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrAppLinkNotFound
	}
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	// sniff version
	var peek struct {
		Version int `json:"ver"`
	}
	if err = json.NewDecoder(fp).Decode(&peek); err != nil {
		return nil, err
	}
	_, _ = fp.Seek(0, io.SeekStart)

	switch peek.Version {
	case 0 /* field missing */ :
		var v0 fileV0
		if err = json.NewDecoder(fp).Decode(&v0); err != nil {
			return nil, err
		}
		return verifyAndOpenV0(key, accountPriv, &v0)

	case 1:
		var v1 fileV1
		if err = json.NewDecoder(fp).Decode(&v1); err != nil {
			return nil, err
		}
		return verifyAndOpenV1(key, accountPriv, &v1)

	default:
		return nil, fmt.Errorf("unsupported version %d", peek.Version)
	}
}

// List reads all app link files in the directory
// For v0 files, only the AppHash field will be populated.
// For v1 files, it will include the whole AppLinkInfo
func list(dir string, accountPriv crypto.PrivKey) ([]*AppLinkInfo, error) {
	// Ensure directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil // Return empty slice if directory doesn't exist
	}

	// Read all .json files in the directory
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}

	var result []*AppLinkInfo
	for _, path := range files {
		// Extract app hash from filename
		filename := filepath.Base(path)
		appHash := strings.TrimSuffix(filename, ".json")

		// Try to read file to determine version
		fp, err := os.Open(path)
		if err != nil {
			continue // Skip files we can't read
		}

		// Attempt to determine version
		var peek struct {
			Version int `json:"ver"`
		}
		if err = json.NewDecoder(fp).Decode(&peek); err != nil {
			fp.Close()
			continue // Skip malformed files
		}

		_, _ = fp.Seek(0, io.SeekStart)

		info := &AppLinkInfo{
			AppHash: appHash,
		}

		// For v1 files, attempt to decrypt if account private key is provided
		if peek.Version == 1 && accountPriv != nil {
			var v1 fileV1
			if err = json.NewDecoder(fp).Decode(&v1); err == nil {
				// Try to decrypt Info field
				if plain, err := accountPriv.Decrypt(v1.Info); err == nil {
					_ = json.Unmarshal(plain, info) // Ignore error, partial data is fine
				}
			}
		}

		fp.Close()
		result = append(result, info)
	}

	return result, nil
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

// fileV1 is the JSON-encoded on-disk structure introduced in format-version 1.
type fileV1 struct {
	// Version is the *file-format* version tag and **must be 1** for this layout.
	// (Future formats should bump this value and add a new struct.)
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

// buildV1 writes the json-ready struct.
func buildV1(appKey []byte, accountPriv crypto.PrivKey, info *AppLinkInfo) (fileV1, error) {
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
	auth := hmacAuth(appKey, ver1, sealed)

	// 3. signature = Ed25519(priv, ver||info||auth)
	sig, err := accountPriv.Sign(bytesForSig(ver1, sealed, auth))
	if err != nil {
		return fileV1{}, err
	}
	return fileV1{
		Version:   ver1,
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
