package wallet

import (
	"bytes"
	"fmt"

	"github.com/anytypeio/go-slip10"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tyler-smith/go-bip39"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-core-wallet")

const (
	AnytypeAccountPrefix = "m/44'/607'"
)

var ErrInvalidWordCount = fmt.Errorf("invalid word count (must be 12, 15, 18, 21, or 24)")

type WordCount int

const (
	TwelveWords     WordCount = 12
	FifteenWords    WordCount = 15
	EighteenWords   WordCount = 18
	TwentyOneWords  WordCount = 21
	TwentyFourWords WordCount = 24
)

func NewWordCount(cnt int) (*WordCount, error) {
	var wc WordCount
	switch cnt {
	case 12:
		wc = TwelveWords
	case 15:
		wc = FifteenWords
	case 18:
		wc = EighteenWords
	case 21:
		wc = TwentyOneWords
	case 24:
		wc = TwentyFourWords
	default:
		return nil, ErrInvalidWordCount
	}
	return &wc, nil
}

func (w WordCount) EntropySize() int {
	switch w {
	case TwelveWords:
		return 128
	case FifteenWords:
		return 160
	case EighteenWords:
		return 192
	case TwentyOneWords:
		return 224
	case TwentyFourWords:
		return 256
	default:
		return 256
	}
}

// Wallet is a BIP32 Hierarchical Deterministic Wallet based on stellar's
// implementation of https://github.com/satoshilabs/slips/blob/master/slip-0010.md,
// https://github.com/stellar/stellar-protocol/pull/63
type Wallet struct {
	RecoveryPhrase string
}

func WalletFromWordCount(wordCount int) (*Wallet, error) {
	wcount, err := NewWordCount(wordCount)
	if err != nil {
		return nil, err
	}

	return WalletFromRandomEntropy(wcount.EntropySize())
}

func WalletFromRandomEntropy(entropySize int) (*Wallet, error) {
	entropy, err := bip39.NewEntropy(entropySize)
	if err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}
	return &Wallet{RecoveryPhrase: mnemonic}, nil
}

func WalletFromMnemonic(mnemonic string) *Wallet {
	return &Wallet{RecoveryPhrase: mnemonic}
}

func WalletFromEntropy(entropy []byte) (*Wallet, error) {
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, err
	}

	return &Wallet{RecoveryPhrase: mnemonic}, nil
}

// To understand how this works, refer to the living document:
// https://paper.dropbox.com/doc/Hierarchical-Deterministic-Wallets--Ae0TOjGObNq_zlyYFh7Ea0jNAQ-t7betWDTvXtK6qqD8HXKf
func (w *Wallet) AccountAt(index int, passphrase string) (Keypair, error) {
	seed, err := bip39.NewSeedWithErrorChecking(w.RecoveryPhrase, passphrase)
	if err != nil {
		if err == bip39.ErrInvalidMnemonic {
			return nil, fmt.Errorf("invalid mnemonic phrase")
		}
		return nil, err
	}
	masterKey, err := slip10.DeriveForPath(AnytypeAccountPrefix, seed)
	if err != nil {
		return nil, err
	}

	key, err := masterKey.Derive(slip10.FirstHardenedIndex + uint32(index))
	if err != nil {
		return nil, err
	}

	reader := bytes.NewReader(key.RawSeed())
	privKey, _, err := crypto.GenerateEd25519Key(reader)

	return NewKeypairFromPrivKey(KeypairTypeAccount, privKey)
}

func (w *Wallet) Entropy() ([]byte, error) {
	return bip39.MnemonicToByteArray(w.RecoveryPhrase, true)
}
