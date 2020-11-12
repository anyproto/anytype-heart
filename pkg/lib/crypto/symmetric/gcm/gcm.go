package gcm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/crypto/symmetric"
)

const (
	// NonceBytes is the length of GCM nonce.
	NonceBytes = 12
)

type GCMEncryptDecryptor struct {
	k symmetric.Key
}

func New(key symmetric.Key) symmetric.EncryptorDecryptor {
	return &GCMEncryptDecryptor{k: key}
}

// Encrypt performs AES-256 GCM encryption on plaintext
func (e *GCMEncryptDecryptor) EncryptReader(r io.Reader) (io.Reader, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	b, err = e.Encrypt(b)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(b), nil
}

// Decrypt uses key to perform AES-256 GCM decryption on ciphertext
func (e *GCMEncryptDecryptor) DecryptReader(r io.ReadSeeker) (symmetric.ReadSeekCloser, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	b, err = e.Decrypt(b)
	if err != nil {
		return nil, err
	}
	rsc := &noopCloser{bytes.NewReader(b)}
	return rsc, nil
}

// Encrypt performs AES-256 GCM encryption on plaintext.
// Encrypt reuses the plaintext arg to save a memory
func (e *GCMEncryptDecryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.k[:symmetric.KeyBytes])
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aesgcm.Seal(plaintext[:0], nonce, plaintext, nil)
	ciphertext = append(nonce[:], ciphertext...)
	return ciphertext, nil
}

// Decrypt uses key to perform AES-256 GCM decryption on ciphertext.
// Decrypt reuses ciphertext arg for the decrypted text to save memory
func (e *GCMEncryptDecryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.k[:symmetric.KeyBytes])
	if err != nil {
		return nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := ciphertext[:NonceBytes]
	ciphertext = ciphertext[NonceBytes:]

	plain, err := aesgcm.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

type noopCloser struct {
	io.ReadSeeker
}

func (nc *noopCloser) Close() error {
	return nil
}
