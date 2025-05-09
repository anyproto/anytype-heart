package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/encode"
)

type assistantConfig struct {
	AccountId string
	Mnemonic  string
	InviteCid string
	InviteKey string
	// SpaceId is fetched from invite automatically, you don't have to write it manually
	SpaceId      string
	SystemPrompt string

	AutoApproveToolUsage bool

	McpServers map[string]mcp.Config
}

func (c *assistantConfig) Validate() error {
	var anyError bool
	if c.AccountId == "" {
		fmt.Println("empty account id")
		anyError = true
	}
	if c.Mnemonic == "" {
		fmt.Println("empty mnemonic")
		anyError = true
	}
	if c.InviteCid == "" {
		fmt.Println("empty invite cid")
		anyError = true
	}
	if c.InviteKey == "" {
		fmt.Println("empty invite key")
		anyError = true
	}
	if c.SystemPrompt == "" {
		fmt.Println("empty system prompt")
		anyError = true
	}
	if anyError {
		return fmt.Errorf("invalid config")
	}
	return nil
}

type testApplication struct {
	appService *application.Service
	account    *model.Account
	eventQueue *mb.MB[*pb.EventMessage]
	config     *assistantConfig
}

func (a *testApplication) personalSpaceId() string {
	return a.account.Info.AccountSpaceId
}

func (a *testApplication) waitEventMessage(t *testing.T, pred func(msg *pb.EventMessage) bool) {
	queueCond := a.eventQueue.NewCond().WithFilter(func(msg *pb.EventMessage) bool {
		return pred(msg)
	})

	queueCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := queueCond.WaitOne(queueCtx)
	require.NoError(t, err)
}

func readConfig(fileName string) (*assistantConfig, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	var config assistantConfig
	decoder := json.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("decode config file: %w", err)
	}
	return &config, nil
}

func writeConfig(fileName string, config *assistantConfig) error {
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(config)
	if err != nil {
		return fmt.Errorf("encode config file: %w", err)
	}
	return nil
}

func createAccountAndStartApp(ctx context.Context, repoDir string, name string, defaultUsecase pb.RpcObjectImportUseCaseRequestUseCase) (*testApplication, error) {
	app := application.New()
	platform := "test"
	version := "1.0.0"
	app.SetClientVersion(platform, version)
	metrics.Service.SetPlatform(platform)
	metrics.Service.SetStartVersion(version)
	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)

	logging.SetLogLevels("WARN")

	resolvedDir := filepath.Join(repoDir, "config.json")
	config, err := readConfig(resolvedDir)
	if errors.Is(err, os.ErrNotExist) {
		return createAccount(ctx, app, repoDir, name, nil, defaultUsecase)
	} else if err != nil {
		return nil, fmt.Errorf("read mnemonic from file: %w", err)
	}
	// If we have a config, but without mnemonic
	if config.Mnemonic == "" {
		return createAccount(ctx, app, repoDir, name, config, defaultUsecase)
	}
	return loadAccount(ctx, app, repoDir, config)
}

func createAccount(ctx context.Context, app *application.Service, repoDir string, name string, config *assistantConfig, defaultUsecase pb.RpcObjectImportUseCaseRequestUseCase) (*testApplication, error) {
	mnemonic, err := app.WalletCreate(&pb.RpcWalletCreateRequest{
		RootPath: repoDir,
	})
	if err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	fmt.Fprintln(os.Stdout, "mnemonic:", mnemonic)

	eventQueue := createEventQueue(ctx, app)

	acc, err := app.AccountCreate(ctx, &pb.RpcAccountCreateRequest{
		Name:        name,
		StorePath:   repoDir,
		NetworkMode: pb.RpcAccount_DefaultConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("account create: %w", err)
	}

	if config == nil {
		config = &assistantConfig{}
	}

	config.AccountId = acc.Id
	config.Mnemonic = mnemonic

	err = writeConfig(filepath.Join(repoDir, "config.json"), config)
	if err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	testApp := &testApplication{
		appService: app,
		account:    acc,
		eventQueue: eventQueue,
		config:     config,
	}
	objCreator := getService[builtinobjects.BuiltinObjects](testApp)
	_, _, err = objCreator.CreateObjectsForUseCase(session.NewContext(), acc.Info.AccountSpaceId, defaultUsecase)
	if err != nil {
		return nil, fmt.Errorf("install usecase: %w", err)
	}
	return testApp, nil
}

func createEventQueue(ctx context.Context, app *application.Service) *mb.MB[*pb.EventMessage] {
	eventQueue := mb.New[*pb.EventMessage](0)
	sender := event.NewCallbackSender(func(event *pb.Event) {
		for _, msg := range event.Messages {
			err := eventQueue.Add(ctx, msg)
			if err != nil {
				log.Error("event queue", zap.Error(err))
			}
		}
	})
	app.SetEventSender(sender)
	return eventQueue
}

func loadAccount(ctx context.Context, app *application.Service, repoDir string, config *assistantConfig) (*testApplication, error) {
	err := app.WalletRecover(&pb.RpcWalletRecoverRequest{
		RootPath: repoDir,
		Mnemonic: config.Mnemonic,
	})
	if err != nil {
		return nil, fmt.Errorf("wallet recover: %w", err)
	}

	eventQueue := createEventQueue(ctx, app)

	acc, err := app.AccountSelect(ctx, &pb.RpcAccountSelectRequest{
		Id:       config.AccountId,
		RootPath: repoDir,
	})
	if err != nil {
		return nil, fmt.Errorf("account select: %w", err)
	}

	testApp := &testApplication{
		appService: app,
		account:    acc,
		eventQueue: eventQueue,
		config:     config,
	}
	return testApp, nil
}

func getService[T any](testApp *testApplication) T {
	a := testApp.appService.GetApp()
	return app.MustComponent[T](a)
}

func joinSpace(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceJoinRequest) (err error) {
	inviteFileKey, err := encode.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	return aclService.Join(ctx, req.SpaceId, req.NetworkId, inviteCid, inviteFileKey)
}

func viewInvite(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceInviteViewRequest) (domain.InviteView, error) {
	inviteFileKey, err := encode.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return domain.InviteView{}, fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return domain.InviteView{}, fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	return aclService.ViewInvite(ctx, inviteCid, inviteFileKey)
}
