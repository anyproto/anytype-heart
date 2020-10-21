package wallet

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalletFromEntropy(t *testing.T) {
	w, err := WalletFromWordCount(12)
	require.NoError(t, err)
	e, err := w.Entropy()
	require.NoError(t, err)
	w2, err := WalletFromEntropy(e)
	require.NoError(t, err)
	require.Equal(t, w.RecoveryPhrase, w2.RecoveryPhrase)
}

func TestNewWordCount(t *testing.T) {
	type args struct {
		cnt int
	}
	tests := []struct {
		name    string
		args    args
		want    *WordCount
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWordCount(tt.args.cnt)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWordCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NewWordCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWordCount_EntropySize(t *testing.T) {
	tests := []struct {
		name string
		w    WordCount
		want int
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.w.EntropySize(); got != tt.want {
				t.Errorf("WordCount.EntropySize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWalletFromWordCount(t *testing.T) {
	type args struct {
		wordCount int
	}
	tests := []struct {
		name              string
		args              args
		wantNumberOfWords int
		wantErr           bool
	}{
		{
			name: "12",
			args: args{
				wordCount: 12,
			},
			wantNumberOfWords: 12,
			wantErr:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := WalletFromWordCount(tt.args.wordCount)
			if (err != nil) != tt.wantErr {
				t.Errorf("WalletFromWordCount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fmt.Println(got.RecoveryPhrase)
			nOfWords := len(strings.Split(got.RecoveryPhrase, " "))
			if !reflect.DeepEqual(nOfWords, tt.wantNumberOfWords) {
				t.Errorf("WalletFromWordCount() = %v, want %v", nOfWords, tt.wantNumberOfWords)
			}
		})
	}
}

func TestWalletFromMnemonic(t *testing.T) {
	type args struct {
		mnemonic string
	}
	tests := []struct {
		name string
		args args
		want *Wallet
	}{
		{
			name: "1",
			args: args{"kitten step voyage hand cover funny timber auction differ mushroom update pulp"},
			want: &Wallet{RecoveryPhrase: "kitten step voyage hand cover funny timber auction differ mushroom update pulp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WalletFromMnemonic(tt.args.mnemonic); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WalletFromMnemonic() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWallet_AccountAt(t *testing.T) {
	type fields struct {
		RecoveryPhrase string
	}
	type args struct {
		index      int
		passphrase string
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		wantRawSeedHex string
		wantSeed       string
		wantAddress    string
		wantErr        bool
	}{
		{
			name: "valid",
			fields: fields{
				RecoveryPhrase: "kitten step voyage hand cover funny timber auction differ mushroom update pulp",
			},
			args: args{
				index:      0,
				passphrase: "",
			},
			wantSeed:       "SWUTakkGB6a5pqf121EJsKt5W4tjt75uV6b8MYyDxLmXUx9S",
			wantRawSeedHex: "a4ee62c1397927e02f227695f364dd6236d0483db3bc1b5bd0ab573000bc87b2ce53148e7d6da0e2b684dc2335a39bd4d01ddaa6b108912b2d4a402327ae8f02",
			wantAddress:    "AAJZ4r91BanPqLAzvdyqdS9asWN5Av8i62vybjxaKULnqdnv",
			wantErr:        false,
		},
		{
			name: "bad phrase",
			fields: fields{
				RecoveryPhrase: "234432",
			},
			args: args{
				index:      0,
				passphrase: "",
			},
			wantSeed:       "",
			wantRawSeedHex: "",
			wantAddress:    "",
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Wallet{
				RecoveryPhrase: tt.fields.RecoveryPhrase,
			}
			got, err := w.AccountAt(tt.args.index, tt.args.passphrase)
			if (err != nil) != tt.wantErr {
				t.Errorf("Wallet.AccountAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			seed := got.Seed()

			rawSeed, err := got.Raw()
			require.NoError(t, err)

			rawSeedHex := fmt.Sprintf("%x", rawSeed)
			if !reflect.DeepEqual(rawSeedHex, tt.wantRawSeedHex) {
				t.Errorf("Wallet.AccountAt() rawSeedHex = %v, want %v", rawSeedHex, tt.wantRawSeedHex)
			}

			if !reflect.DeepEqual(seed, tt.wantSeed) {
				t.Errorf("Wallet.AccountAt() seed = %v, want %v", seed, tt.wantSeed)
			}

			if !reflect.DeepEqual(got.Address(), tt.wantAddress) {
				t.Errorf("Wallet.AccountAt() Address = %v, want %v", got.Address(), tt.wantAddress)
			}
		})
	}
}
