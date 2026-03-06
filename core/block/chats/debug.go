package chats

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/messages/{chatObjectId}", debug.JSONHandler(s.debugMessages))
}

func (s *service) debugMessages(req *http.Request) ([]json.RawMessage, error) {
	chatObjectId := chi.URLParam(req, "chatObjectId")

	spaceId, err := s.spaceIdResolver.ResolveSpaceID(chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("resolve space id: %w", err)
	}

	repo, err := s.chatRepoService.Repository(spaceId, chatObjectId)
	if err != nil {
		return nil, fmt.Errorf("get repository: %w", err)
	}

	messages, err := repo.GetAllRawMessages(req.Context())
	if err != nil {
		return nil, fmt.Errorf("get all raw messages: %w", err)
	}

	return messages, nil
}
