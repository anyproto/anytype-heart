package wallet

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	json "encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric/gcm"
)

const appLinkKeysDirectory = "auth"

var ErrAppLinkNotFound = fmt.Errorf("app link file not found in the account directory")

type AppLinkPayload struct {
	AccountPrivateKey []byte `json:"key"` // stored only for the main client app
	AppName           string `json:"app_name"`
	AppPath           string `json:"app_path"`   // for now, it is not verified
	CreatedAt         int64  `json:"created_at"` // unix timestamp
	ExpireAt          int64  `json:"expire_at"`  // unix timestamp
	Scope             int    `json:"scope"`
}

type appLinkFileEncrypted struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"` // account signature to check the integrity of the payload
}

func writeAppLinkFile(accountPk crypto.PrivKey, dir string, payload *AppLinkPayload) (appKey string, err error) {
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
	encryptedPayload := appLinkFileEncrypted{
		Payload:   encPayload,
		Signature: signature,
	}

	hash := sha256.Sum256(key.Bytes())
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%x", hash)+".json"))
	if err != nil {
		return "", fmt.Errorf("failed to create app key file in the account: %w", err)
	}
	// todo: change perms?

	w := json.NewEncoder(f)
	err = w.Encode(encryptedPayload)
	if err != nil {
		return "", err
	}
	return appKey, nil
}

func readAppLinkFile(pubKey crypto.PubKey, dir string, appKey string) (*AppLinkPayload, error) {
	symKeyBytes, err := base64.StdEncoding.DecodeString(appKey)

	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(symKeyBytes)
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%x", hash)+".json"))
	if err != nil {
		// in case it is not found, we return a special error
		if os.IsNotExist(err) {
			return nil, ErrAppLinkNotFound
		}
		return nil, err
	}
	defer f.Close()

	appKeyFile := appLinkFileEncrypted{}
	d := json.NewDecoder(f)
	err = d.Decode(&appKeyFile)

	if err != nil {
		return nil, err
	}

	err = signatureVerify(pubKey, &appKeyFile)
	if err != nil {
		return nil, err
	}

	key, err := symmetric.FromBytes(symKeyBytes)
	if err != nil {
		return nil, err
	}

	r, err := gcm.New(key).DecryptReader(bytes.NewReader(appKeyFile.Payload))
	if err != nil {
		return nil, err
	}

	d = json.NewDecoder(r)
	payload := AppLinkPayload{}
	err = d.Decode(&payload)
	if err != nil {
		return nil, err
	}

	return &payload, nil
}

func signatureVerify(key crypto.PubKey, encrypted *appLinkFileEncrypted) error {
	v, err := key.Verify(encrypted.Payload, encrypted.Signature)
	if err != nil {
		return err
	}
	if !v {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func (r *wallet) ReadAppLink(appKey string) (*AppLinkPayload, error) {
	if r.repoPath == "" {
		return nil, fmt.Errorf("repo path is not set")
	}

	if r.accountKey == nil {
		return nil, fmt.Errorf("account is not set")
	}

	// readAppLinkFile verifies the signature of the payload to make sure it was not tampered and the account id matches
	return readAppLinkFile(r.accountKey.GetPublic(), filepath.Join(r.repoPath, appLinkKeysDirectory), appKey)
}

func (r *wallet) PersistAppLink(payload *AppLinkPayload) (appKey string, err error) {
	return writeAppLinkFile(r.accountKey, filepath.Join(r.repoPath, appLinkKeysDirectory), payload)
}

func DeriveForAccount(seed []byte) (res crypto.DerivationResult, err error) {
	res.Identity = crypto.NewEd25519PrivKey(ed25519.NewKeyFromSeed(seed))
	return
}
