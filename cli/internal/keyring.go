package internal

import "github.com/zalando/go-keyring"

const (
	// keyringService is the identifier for the CLI in the OS keychain.
	keyringService = "anytype-cli"
	// keyringUser is the key under which the mnemonic is stored.
	keyringUser = "mnemonic"
)

// SaveMnemonic stores the mnemonic securely in the OS keychain.
func SaveMnemonic(mnemonic string) error {
	return keyring.Set(keyringService, keyringUser, mnemonic)
}

// GetStoredMnemonic retrieves the mnemonic from the OS keychain.
func GetStoredMnemonic() (string, error) {
	return keyring.Get(keyringService, keyringUser)
}

// DeleteStoredMnemonic removes the mnemonic from the OS keychain.
func DeleteStoredMnemonic() error {
	return keyring.Delete(keyringService, keyringUser)
}
