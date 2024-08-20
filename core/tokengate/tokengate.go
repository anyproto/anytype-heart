package tokengate

import (
	"context"
	"errors"
	"math/big"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"go.uber.org/zap"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/anyproto/anytype-heart/core/wallet"
)

const CName = "tokengate"

var log = logging.Logger(CName).Desugar()

var (
	ErrBadNftTokenAddr = errors.New("bad NFT token address")
	ErrNotAnNftOwner   = errors.New("you do not have a required NFT to join that space")
)

type TokenGatingService interface {
	CheckNftOwnership(ctx context.Context, tokenAddr string) error

	app.Component
}

func New() TokenGatingService {
	return &service{}
}

type service struct {
	wallet wallet.Wallet
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.wallet = app.MustComponent[wallet.Wallet](a)
	return nil
}

func getNFTBalance(client *ethclient.Client, contractAddress, userAddress common.Address, tokenID *big.Int) (*big.Int, error) {
	// Create a new instance of the ERC1155 contract
	contract, err := NewERC1155(contractAddress, client)
	if err != nil {
		return nil, err
	}

	// Query the `balanceOf` function
	balance, err := contract.BalanceOf(&bind.CallOpts{
		Context: context.Background(),
	}, userAddress, tokenID)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// The ERC1155 ABI (replace with the correct ABI for your NFT contract)
const erc1155ABI = `[{"constant":true,"inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"}]`

// NewERC1155 creates a new instance of an ERC1155 contract
func NewERC1155(address common.Address, client *ethclient.Client) (*ERC1155, error) {
	parsed, err := abi.JSON(strings.NewReader(erc1155ABI))
	if err != nil {
		return nil, err
	}
	return &ERC1155{
		Contract: bind.NewBoundContract(address, parsed, client, client, client),
	}, nil
}

// ERC1155 is a Go wrapper around an on-chain ERC1155 contract.
type ERC1155 struct {
	Contract *bind.BoundContract
}

// BalanceOf is a free data retrieval call binding the contract method 0x00fdd58e.
//
// Solidity: function balanceOf(address account, uint256 id) view returns(uint256)
func (c *ERC1155) BalanceOf(opts *bind.CallOpts, account common.Address, id *big.Int) (*big.Int, error) {
	var out []interface{}
	err := c.Contract.Call(opts, &out, "balanceOf", account, id)
	if err != nil {
		return nil, err
	}
	return out[0].(*big.Int), nil
}

// TODO: implement
func (s *service) CheckNftOwnership(ctx context.Context, tokenAddr string) error {
	log.Debug("checking NFT ownership", zap.String("tokenAddr", tokenAddr))

	// 1 - get my Eth address from Anytype wallet
	userAddr := s.wallet.GetAccountEthAddress()

	// https://testnets.opensea.io/collection/local-first-webdev-club-berlin
	//if tokenAddr == "0x709981f628593C60182F77F15abC59BC47609d13" {
	// allow for test
	//return nil

	infuraURL := "https://sepolia.infura.io/v3/314726ab89c54e6fa530f6323b48ac67"
	nftContract := common.HexToAddress(tokenAddr)

	// TODO: for debug only
	//nftContract := common.HexToAddress("0x709981f628593c60182f77f15abc59bc47609d13")

	// Connect to the Ethereum network
	client, err := ethclient.Dial(infuraURL)
	if err != nil {
		log.Error("Failed to connect to the Ethereum network", zap.Error(err))
		return err
	}

	// TODO:
	// Iterate through a range of token IDs (0 to 19)
	for i := 0; i < 20; i++ {
		tokenID := big.NewInt(int64(i))

		// Call the `balanceOf` function of the ERC1155 contract
		balance, err := getNFTBalance(client, nftContract, userAddr, tokenID)
		if err != nil {
			log.Error("Failed to get NFT balance", zap.Error(err))
			continue
		}

		// Check if the balance is greater than 0
		if balance.Cmp(big.NewInt(0)) > 0 {
			log.Info("NFT owner found!", zap.String("tokenAddr", tokenAddr), zap.Int("tokenID", i))
			return nil
		}
	}

	return ErrNotAnNftOwner
}
