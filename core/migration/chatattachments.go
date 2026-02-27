package migration

import (
	"context"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ChatAttachmentContext represents a file found in a chat message
type ChatAttachmentContext struct {
	ChatObjectId string
	MessageId    string
	CreatedAt    int64
}

// ChatAttachmentIndex maps fileId to its earliest chat attachment context
type ChatAttachmentIndex map[string]*ChatAttachmentContext

// buildChatAttachmentIndex scans all chats in a space once and builds an index of fileId -> earliest ChatAttachmentContext
func (s *service) buildChatAttachmentIndex(ctx context.Context, spaceId string, spaceIndex spaceindex.Store) (ChatAttachmentIndex, error) {
	index := make(ChatAttachmentIndex)

	chatObjectIds, err := s.getChatObjectIds(spaceIndex)
	if err != nil {
		return nil, err
	}

	if len(chatObjectIds) == 0 {
		return index, nil
	}

	for _, chatObjectId := range chatObjectIds {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		repo, err := s.chatRepository.Repository(spaceId, chatObjectId)
		if err != nil {
			log.Debug("error getting chat repository",
				zap.String("chatObjectId", chatObjectId),
				zap.Error(err))
			continue
		}

		attachments, err := repo.GetAllMessageAttachments(ctx, "")
		if err != nil {
			log.Debug("error getting chat attachments",
				zap.String("chatObjectId", chatObjectId),
				zap.Error(err))
			continue
		}

		for _, att := range attachments {
			for _, fileId := range att.FileIds {
				existing, exists := index[fileId]
				if !exists || att.CreatedAt < existing.CreatedAt {
					index[fileId] = &ChatAttachmentContext{
						ChatObjectId: chatObjectId,
						MessageId:    att.MessageId,
						CreatedAt:    att.CreatedAt,
					}
				}
			}
		}
	}

	return index, nil
}

// getChatObjectIds queries for all chat objects in the space
func (s *service) getChatObjectIds(spaceIndex spaceindex.Store) ([]string, error) {
	chatRecords, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_chatDerived)),
		}},
	})
	if err != nil {
		return nil, err
	}

	chatObjectIds := make([]string, 0, len(chatRecords))
	for _, rec := range chatRecords {
		chatObjectIds = append(chatObjectIds, rec.Details.GetString(bundle.RelationKeyId))
	}
	return chatObjectIds, nil
}
