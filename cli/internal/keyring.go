package internal

import "github.com/zalando/go-keyring"

const (
	// keyringService is the identifier for the CLI in the OS keychain.
	keyringService = "anytype-cli"
	// keyringMnemonicUser is the key under which the mnemonic is stored.
	keyringMnemonicUser = "mnemonic"
	// keyringTokenUser is the key under which the session token is stored.
	keyringTokenUser = "session-token"
)

// SaveMnemonic stores the mnemonic securely in the OS keychain.
func SaveMnemonic(mnemonic string) error {
	return keyring.Set(keyringService, keyringMnemonicUser, mnemonic)
}

// GetStoredMnemonic retrieves the mnemonic from the OS keychain.
func GetStoredMnemonic() (string, error) {
	return keyring.Get(keyringService, keyringMnemonicUser)
}

// DeleteStoredMnemonic removes the mnemonic from the OS keychain.
func DeleteStoredMnemonic() error {
	return keyring.Delete(keyringService, keyringMnemonicUser)
}

// SaveToken stores the session token securely in the OS keychain.
func SaveToken(token string) error {
	return keyring.Set(keyringService, keyringTokenUser, token)
}

// GetStoredToken retrieves the session token from the OS keychain.
func GetStoredToken() (string, error) {
	return keyring.Get(keyringService, keyringTokenUser)
}

// DeleteStoredToken removes the session token from the OS keychain.
func DeleteStoredToken() error {
	return keyring.Delete(keyringService, keyringTokenUser)
}
