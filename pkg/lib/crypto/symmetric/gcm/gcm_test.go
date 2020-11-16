package gcm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/crypto/symmetric"
	"github.com/stretchr/testify/require"
)

var symmetricTestData = struct {
	key        symmetric.Key
	plaintext  []byte
	ciphertext []byte
}{
	plaintext: []byte("1234567890qwertyuiopasdfghjklzxcvbnm1234567890qwertyuiopasdfghjklzxcvbnm1234567890qwertyuiopasdfghjk"),
}

func TestNewRandom(t *testing.T) {
	k, err := symmetric.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	symmetricTestData.key = k
}

func TestEncryptReader(t *testing.T) {
	r := bytes.NewReader(symmetricTestData.plaintext)

	d := New(symmetricTestData.key)
	ciphertext, err := d.EncryptReader(r)
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	symmetricTestData.ciphertext = b
}

func TestDecryptReader(t *testing.T) {
	t.Run("correct key", func(t *testing.T) {
		cipherReader := bytes.NewReader(symmetricTestData.ciphertext)

		d := New(symmetricTestData.key)

		plaintextReader, err := d.DecryptReader(cipherReader)
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(plaintextReader)
		if err != nil {
			t.Fatal(err)
		}

		if string(symmetricTestData.plaintext) != string(b) {
			t.Error("decrypt AES failed: ", string(b))
		}
	})

	t.Run("incorrect key", func(t *testing.T) {
		cipherReader := bytes.NewReader(symmetricTestData.ciphertext)
		key, err := symmetric.NewRandom()
		if err != nil {
			t.Fatal(err)
		}

		d := New(key)

		_, err = d.DecryptReader(cipherReader)
		require.Error(t, err)
	})

	t.Run("seek", func(t *testing.T) {
		var seekTests = []struct {
			offset int64
			whence int
			length int64
		}{
			{0, io.SeekStart, 16},
			{0, io.SeekStart, 16},
			{0, io.SeekStart, 10},
			{0, io.SeekStart, 20},
			{16, io.SeekStart, 16},
			{16, io.SeekStart, 10},
			{16, io.SeekStart, 21},
			{21, io.SeekStart, 11},
			{21, io.SeekStart, 16},
			{21, io.SeekStart, 21},
			{0, io.SeekCurrent, 16},
			{0, io.SeekCurrent, 10},
			{0, io.SeekEnd, 16},
			{0, io.SeekEnd, 10},
			{0, io.SeekEnd, 20},
			{-16, io.SeekEnd, 16},
			{-16, io.SeekEnd, 10},
			{-16, io.SeekEnd, 21},
			{-21, io.SeekEnd, 11},
			{-21, io.SeekEnd, 16},
			{-21, io.SeekEnd, 21},
			{96, io.SeekStart, 4},
			{96, io.SeekStart, 21},
			{101, io.SeekStart, 21},
		}

		plaintextReader := bytes.NewReader(symmetricTestData.plaintext)
		d := New(symmetricTestData.key)
		decryptedReader, err := d.DecryptReader(bytes.NewReader(symmetricTestData.ciphertext))
		if err != nil {
			t.Fatal(err)
		}

		for i, st := range seekTests {
			t.Run(
				fmt.Sprintf("seek_test_%d", i),
				func(t *testing.T) {
					dOffset, dErr := decryptedReader.Seek(st.offset, st.whence)
					pOffset, pErr := plaintextReader.Seek(st.offset, st.whence)

					require.Equal(t, pOffset, dOffset)
					require.Equal(t, pErr, dErr)

					dB := make([]byte, st.length)
					pB := make([]byte, st.length)

					dN, dErr := io.ReadFull(decryptedReader, dB)
					pN, pErr := io.ReadFull(plaintextReader, pB)
					require.Equal(t, pErr, dErr)
					require.Equal(t, pN, dN)
					require.Equal(t, string(pB), string(dB))
				})
		}

	})
}
