package migration

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
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

	// Step 1: Get all chat object IDs in the space
	chatObjectIds, err := s.getChatObjectIds(spaceIndex)
	if err != nil {
		return nil, err
	}

	if len(chatObjectIds) == 0 {
		return index, nil
	}

	crdtDb, err := s.dbProvider.GetCrdtDb(spaceId).Wait()
	if err != nil {
		return nil, err
	}

	// Step 2: Scan each chat's messages and extract all attachments
	for _, chatObjectId := range chatObjectIds {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		collectionName := chatObjectId + "chats"
		collection, err := crdtDb.OpenCollection(ctx, collectionName)
		if errors.Is(err, anystore.ErrCollectionNotFound) {
			continue
		}
		if err != nil {
			log.Debug("error opening chat collection",
				zap.String("chatObjectId", chatObjectId),
				zap.Error(err))
			continue
		}

		// Scan all messages in this chat
		iter, err := collection.Find(nil).Iter(ctx)
		if err != nil {
			log.Debug("error iterating chat messages",
				zap.String("chatObjectId", chatObjectId),
				zap.Error(err))
			continue
		}

		for iter.Next() {
			doc, err := iter.Doc()
			if err != nil {
				continue
			}

			val := doc.Value()
			messageId := val.GetString("id")
			createdAt := int64(val.GetInt("createdAt"))

			// Get attachments object
			attachments := val.GetObject("content", "attachments")
			if attachments == nil {
				continue
			}

			// Visit each attachment (key is fileId)
			attachments.Visit(func(fileIdBytes []byte, _ *anyenc.Value) {
				fileId := string(fileIdBytes)

				existing, exists := index[fileId]
				if !exists || createdAt < existing.CreatedAt {
					index[fileId] = &ChatAttachmentContext{
						ChatObjectId: chatObjectId,
						MessageId:    messageId,
						CreatedAt:    createdAt,
					}
				}
			})
		}
		iter.Close()
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
