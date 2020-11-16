package cfb

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/crypto/symmetric"
)

type Sizable interface {
	Size() uint64
}

type CFBEncryptDecryptor struct {
	k  symmetric.Key
	iv [aes.BlockSize]byte
}

type CFBDecryptor struct {
	k          symmetric.Key
	block      cipher.Block
	sr         *cipher.StreamReader
	iv         [aes.BlockSize]byte
	currOffset int64
}

func New(key symmetric.Key, iv [aes.BlockSize]byte) symmetric.EncryptorDecryptor {
	return &CFBEncryptDecryptor{k: key, iv: iv}
}

// Encrypt performs AES-256 CFB encryption on plaintext.
func (e *CFBEncryptDecryptor) EncryptReader(r io.Reader) (io.Reader, error) {
	block, err := aes.NewCipher(e.k[:symmetric.KeyBytes])
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCFBEncrypter(block, e.iv[:])

	return &cipher.StreamReader{S: stream, R: r}, nil
}

// Decrypt uses key to perform AES-256 CFB decryption on ciphertext.
func (e *CFBEncryptDecryptor) DecryptReader(r io.ReadSeeker) (symmetric.ReadSeekCloser, error) {
	block, err := aes.NewCipher(e.k[:symmetric.KeyBytes])
	if err != nil {
		return nil, err
	}

	d := &CFBDecryptor{k: e.k, iv: e.iv}
	stream := cipher.NewCFBDecrypter(block, d.iv[:])
	d.sr = &cipher.StreamReader{S: stream, R: r}
	d.block = block

	return d, nil
}

func (d *CFBDecryptor) Seek(offset int64, whence int) (int64, error) {
	var (
		cipherTextReader io.ReadSeeker
		ok               bool
	)

	if cipherTextReader, ok = d.sr.R.(io.ReadSeeker); !ok {
		return 0, fmt.Errorf("underlying cipher reader is not seekable")
	}

	var newoffset int64
	switch whence {
	case io.SeekCurrent:
		newoffset = d.currOffset + offset
	case io.SeekStart:
		newoffset = offset
	case io.SeekEnd:
		if sizable, ok := cipherTextReader.(Sizable); ok {
			newoffset = int64(sizable.Size()) + offset
		} else {
			var err error
			newoffset, err = cipherTextReader.Seek(offset, whence)
			if err != nil {
				return 0, err
			}
		}
	default:
		return 0, fmt.Errorf("unrecognized whence")
	}

	if sizable, ok := cipherTextReader.(Sizable); ok && newoffset > int64(sizable.Size()) {
		return 0, fmt.Errorf("offset out of range")
	}

	correctedOffset := (newoffset / aes.BlockSize) * aes.BlockSize
	if correctedOffset >= aes.BlockSize {
		// seek to prev block to get IV
		_, err := cipherTextReader.Seek(correctedOffset-aes.BlockSize, io.SeekStart)
		if err != nil {
			return 0, fmt.Errorf("failed to seek for previous block to get IV: %w", err)
		}

		var iv = make([]byte, aes.BlockSize)
		_, err = io.ReadFull(cipherTextReader, iv)
		if err != nil {
			return 0, fmt.Errorf("failed to read the previous block to get IV: %w", err)
		}
		copy(d.iv[:], iv[:aes.BlockSize])
	} else {
		d.iv = [aes.BlockSize]byte{}
		i, err := cipherTextReader.Seek(correctedOffset, io.SeekStart)
		if err != nil {
			return 0, fmt.Errorf("failed to seek underlying reader to the offset: %w", err)
		}
		if i != correctedOffset {
			return 0, fmt.Errorf("failed to seek underlying reader to the offset: result offset mismatch %d != %d", i, correctedOffset)
		}
	}
	stream := cipher.NewCFBDecrypter(d.block, d.iv[:])
	d.sr.S = stream
	// skip corrected bytes so all corresponding Reads will match the Seek
	bytesToSkip := newoffset - correctedOffset
	if bytesToSkip > 0 {
		_, err := io.CopyN(ioutil.Discard, d.sr, newoffset-correctedOffset)
		if err != nil {
			return 0, err
		}
	}

	d.currOffset = newoffset
	return newoffset, nil
}

func (r *CFBDecryptor) Read(b []byte) (n int, err error) {
	n, err = r.sr.Read(b)
	r.currOffset += int64(n)
	return
}

func (r *CFBDecryptor) Close() error {
	if c, ok := r.sr.R.(io.Closer); ok {
		return c.Close()
	}

	return nil
}
