package cfb

import (
	"bytes"
	"crypto/aes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
)

// sizableReader wraps bytes.Reader and adds the Sizable interface.
// It tracks whether Seek was called on the underlying reader.
type sizableReader struct {
	*bytes.Reader
	size      uint64
	seekCount int
}

func newSizableReader(data []byte) *sizableReader {
	return &sizableReader{
		Reader: bytes.NewReader(data),
		size:   uint64(len(data)),
	}
}

func (s *sizableReader) Size() uint64 {
	return s.size
}

func (s *sizableReader) Seek(offset int64, whence int) (int64, error) {
	s.seekCount++
	return s.Reader.Seek(offset, whence)
}

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

	d := New(symmetricTestData.key, [aes.BlockSize]byte{})
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

		d := New(symmetricTestData.key, [aes.BlockSize]byte{})

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

		d := New(key, [aes.BlockSize]byte{})

		plaintextReader, err := d.DecryptReader(cipherReader)
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(plaintextReader)
		if err != nil {
			t.Fatal(err)
		}

		if string(symmetricTestData.plaintext) == string(b) {
			t.Error("decrypt with incorrect key should provide incorrect result")
		}
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
		d := New(symmetricTestData.key, [aes.BlockSize]byte{})
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

					if pOffset > int64(len(symmetricTestData.plaintext)) {
						// in case we out of bounds it is ok for our implementation to return error
						// bytes reader seek instead will postpone the error until the actual read
						require.Error(t, dErr)
						return
					}

					require.Equal(t, pErr, dErr)
					if pErr != nil {
						return
					}

					require.Equal(t, pOffset, dOffset)

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

func TestSeekEndSizableFastPath(t *testing.T) {
	// Set up encryption
	key, err := symmetric.NewRandom()
	require.NoError(t, err)
	iv := [aes.BlockSize]byte{}

	plaintext := bytes.Repeat([]byte("abcdefghijklmnop"), 100) // 1600 bytes
	enc := New(key, iv)

	ciphertextReader, err := enc.EncryptReader(bytes.NewReader(plaintext))
	require.NoError(t, err)
	ciphertext, err := ioutil.ReadAll(ciphertextReader)
	require.NoError(t, err)

	t.Run("SeekEnd does not seek underlying sizable reader", func(t *testing.T) {
		sr := newSizableReader(ciphertext)
		dec, err := New(key, iv).DecryptReader(sr)
		require.NoError(t, err)

		sr.seekCount = 0

		// Seek(0, SeekEnd) should return size without seeking underlying reader
		pos, err := dec.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(len(ciphertext)), pos)
		assert.Equal(t, 0, sr.seekCount, "Seek(0, SeekEnd) should not seek underlying sizable reader")
	})

	t.Run("SeekEnd then SeekStart then Read produces correct data", func(t *testing.T) {
		// This simulates what http.ServeContent does:
		// 1. Seek(0, SeekEnd) to get size
		// 2. Seek(0, SeekStart) to reset
		// 3. Read the data
		sr := newSizableReader(ciphertext)
		dec, err := New(key, iv).DecryptReader(sr)
		require.NoError(t, err)

		// Step 1: size determination
		size, err := dec.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(len(plaintext)), size)

		// Step 2: reset to start
		pos, err := dec.Seek(0, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, int64(0), pos)

		// Step 3: read all
		got, err := ioutil.ReadAll(dec)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	})

	t.Run("SeekEnd then range seek then Read produces correct data", func(t *testing.T) {
		// Simulates http.ServeContent with Range header:
		// 1. Seek(0, SeekEnd) to get size
		// 2. Seek(rangeStart, SeekStart) to position
		// 3. Read the range
		sr := newSizableReader(ciphertext)
		dec, err := New(key, iv).DecryptReader(sr)
		require.NoError(t, err)

		// Step 1: size determination
		size, err := dec.Seek(0, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(len(plaintext)), size)

		// Step 2: seek to mid-file range
		rangeStart := int64(100)
		pos, err := dec.Seek(rangeStart, io.SeekStart)
		require.NoError(t, err)
		assert.Equal(t, rangeStart, pos)

		// Step 3: read range
		rangeLen := 200
		buf := make([]byte, rangeLen)
		n, err := io.ReadFull(dec, buf)
		require.NoError(t, err)
		assert.Equal(t, rangeLen, n)
		assert.Equal(t, plaintext[rangeStart:rangeStart+int64(rangeLen)], buf)
	})

	t.Run("SeekEnd with negative offset seeks underlying reader", func(t *testing.T) {
		sr := newSizableReader(ciphertext)
		dec, err := New(key, iv).DecryptReader(sr)
		require.NoError(t, err)

		sr.seekCount = 0

		// Non-zero offset must go through the full cipher setup path
		// so that Read after Seek produces correct data
		pos, err := dec.Seek(-16, io.SeekEnd)
		require.NoError(t, err)
		assert.Equal(t, int64(len(ciphertext))-16, pos)
		assert.Greater(t, sr.seekCount, 0, "Seek(-16, SeekEnd) should seek underlying reader to set up cipher state")

		// Verify reading produces correct plaintext
		buf := make([]byte, 16)
		n, err := io.ReadFull(dec, buf)
		require.NoError(t, err)
		assert.Equal(t, 16, n)
		assert.Equal(t, plaintext[len(plaintext)-16:], buf)
	})

	t.Run("SeekEnd out of range returns error", func(t *testing.T) {
		sr := newSizableReader(ciphertext)
		dec, err := New(key, iv).DecryptReader(sr)
		require.NoError(t, err)

		_, err = dec.Seek(1, io.SeekEnd)
		assert.Error(t, err)

		_, err = dec.Seek(-int64(len(ciphertext))-1, io.SeekEnd)
		assert.Error(t, err)
	})
}
