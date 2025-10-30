package onetoone

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/core/inboxclient"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
)

const CName = "heart.onetoone"

var log = logger.NewNamed(CName)

var (
	ErrSomeError = errors.New("some error")
)

func New() Service {
	return new(onetoone)
}

type Service interface {
	app.ComponentRunnable
	SendOneToOneInvite(ctx context.Context, idWithProfileKey *model.IdentityProfileWithKey) (err error)
}

type BlockService interface {
	CreateWorkspace(ctx context.Context, req *pb.RpcWorkspaceCreateRequest) (spaceID string, startingPageId string, err error)
	SpaceInitChat(ctx context.Context, spaceId string) error
}
type onetoone struct {
	inboxClient  inboxclient.InboxClient
	blockService BlockService
}

func (s *onetoone) Init(a *app.App) (err error) {
	s.blockService = app.MustComponent[BlockService](a)
	s.inboxClient = app.MustComponent[inboxclient.InboxClient](a)
	err = s.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, s.processOneToOneInvite)
	if err != nil {
		log.Error("failed to init inbox receiver", zap.Error(err))
		return err
	}

	return
}

func (s *onetoone) processOneToOneInvite(packet *coordinatorproto.InboxPacket) (err error) {
	inboxBody := packet.Payload.Body

	if inboxBody == nil {
		return fmt.Errorf("processOneToOneInvite: got nil body")
	}

	var idWithProfileKey model.IdentityProfile
	err = proto.Unmarshal(inboxBody, &idWithProfileKey)
	if err != nil {
		return
	}
	log.Debug("creating onetoone space from inbox.. ")

	req := &pb.RpcWorkspaceCreateRequest{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeySpaceUxType.String():      pbtypes.Float64(float64(model.SpaceUxType_OneToOne)),
				bundle.RelationKeyName.String():             pbtypes.String(idWithProfileKey.Name),
				bundle.RelationKeyIconOption.String():       pbtypes.Float64(float64(5)),
				bundle.RelationKeySpaceDashboardId.String(): pbtypes.String("lastOpened"),
				bundle.RelationKeyOneToOneIdentity.String(): pbtypes.String(idWithProfileKey.Identity),
			},
		},
		UseCase:  pb.RpcObjectImportUseCaseRequest_CHAT_SPACE,
		WithChat: true,
	}

	//TODO: lol, you forgot to put RegisterIdentity()
	spaceId, _, err := s.blockService.CreateWorkspace(context.TODO(), req)
	if err != nil {
		return
	}
	err = s.blockService.SpaceInitChat(context.TODO(), spaceId)
	if err != nil {
		log.Warn("failed to init space level chat")
	}

	log.Debug("created onetoone space from inbox", zap.String("spaceId", spaceId))

	// TODO: RegisterParticipant
	// TODO: cyclic deps. we can create a third service, or, pass processOneToOneInvite from space service.
	// or, register notifiers for each type..?
	// s.spaceService.Create
	// createOneToOne

	return err
}
func (s *onetoone) Name() (name string) {
	return CName
}

func (s *onetoone) Run(ctx context.Context) error {
	return nil
}

func (s *onetoone) Close(_ context.Context) (err error) {
	return nil
}

// 1. wrap identity and request metadata key to payload
// 2. change spaceinvite to onetoone request
// 3. auto accept inbox
// 4. don't send inbox if space already exists (check spaceveiw)
// 5. on inbox accept, skip creation if space exist
func (s *onetoone) SendOneToOneInvite(ctx context.Context, idWithProfileKey *model.IdentityProfileWithKey) (err error) {
	// 1. put whole identity profile
	// 2. try to get this from WaitProfile or register this incoming identity
	// 3. createOneToOne(bPk)
	body, err := idWithProfileKey.Marshal()
	if err != nil {
		return
	}

	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: idWithProfileKey.IdentityProfile.Identity,
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
			},
		},
	}

	participantPubKey, err := crypto.DecodeAccountAddress(idWithProfileKey.IdentityProfile.Identity)
	if err != nil {
		return
	}

	return s.inboxClient.InboxAddMessage(ctx, participantPubKey, msg)
}
