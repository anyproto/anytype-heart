package account

import (
	"github.com/anyproto/anytype-heart/pb"
	"fmt"
	"github.com/anyproto/anytype-heart/core/domain"
	"encoding/base64"
	"github.com/anyproto/any-sync/util/crypto"
)

// WalletConvert converts mnemonic to base64 representation of mnemonic's entropy and vice versa.
// Entropy is used to generate QR code
// This QR code is scanned by mobile app and the entropy is extracted from it is converted to mnemonic
func (s *Service) WalletConvert(req *pb.RpcWalletConvertRequest) (mnemonicString string, base64Entropy string, err error) {
	if req.Mnemonic == "" && req.Entropy != "" {
		b, err := base64.RawStdEncoding.DecodeString(req.Entropy)
		if err != nil {
			return "", "", domain.WrapErrorWithCode(fmt.Errorf("invalid base64 format for entropy: %w", err), pb.RpcWalletConvertResponseError_BAD_INPUT)
		}
		mnemonic, err := crypto.NewMnemonicGenerator().WithEntropy(b)
		if err != nil {
			return "", "", domain.WrapErrorWithCode(fmt.Errorf("invalid entropy: %w", err), pb.RpcWalletConvertResponseError_BAD_INPUT)
		}
		return string(mnemonic), "", nil
	} else if req.Mnemonic != "" && req.Entropy == "" {
		mnemonic := crypto.Mnemonic(req.Mnemonic)
		entropy, err := mnemonic.Bytes()
		if err != nil {
			return "", "", domain.WrapErrorWithCode(err, pb.RpcWalletConvertResponseError_BAD_INPUT)
		}

		base64Entropy = base64.RawStdEncoding.EncodeToString(entropy)
		return "", base64Entropy, nil
	}

	return "", "", domain.WrapErrorWithCode(fmt.Errorf("you should specify either entropy or mnemonic to convert"), pb.RpcWalletConvertResponseError_BAD_INPUT)
}
