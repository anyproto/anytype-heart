package wallet

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	rand2 "math/rand"
	"os"
	"testing"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"
)

func Test_writeAppLinkFile(t *testing.T) {
	// 10 to the 4th power
	a := 4

	fmt.Println(a)
	value := fmt.Sprintf("%0*d", a, rand2.Intn(int(math.Pow10(a))))
	fmt.Println(value)

}
func Test_writeAppLinkFileAndRead(t *testing.T) {
	priv, pub, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	tempDir := os.TempDir()
	payload := &AppLinkPayload{
		AppName:   "test",
		AppPath:   "path",
		CreatedAt: time.Now().Unix(),
		ExpireAt:  0,
	}
	gotAppKey, err := writeAppLinkFile(priv, tempDir, payload)
	require.NoError(t, err)

	require.NotEmpty(t, gotAppKey)

	gotPayload, err := readAppLinkFile(pub, tempDir, gotAppKey)
	require.NoError(t, err)
	require.Equal(t, payload, gotPayload)
}

func Test_signatureVerify(t *testing.T) {
	priv, pub, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)

	data := make([]byte, 100)
	_, err = rand.Reader.Read(data)
	require.NoError(t, err)

	sign, err := priv.Sign(data)
	require.NoError(t, err)

	payload := &appLinkFileEncrypted{Payload: data, Signature: sign}

	err = signatureVerify(pub, payload)
	require.NoError(t, err)

	jb, err := json.Marshal(payload)
	require.NoError(t, err)

	payload2 := &appLinkFileEncrypted{}
	err = json.Unmarshal(jb, payload2)
	require.NoError(t, err)
	err = signatureVerify(pub, payload2)
	require.NoError(t, err)
}
