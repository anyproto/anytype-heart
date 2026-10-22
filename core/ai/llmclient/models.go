package llmclient

import (
	"context"
	"fmt"
	"strings"
)

// Model is one catalog entry from a provider's GET /v1/models, trimmed to
// what a client-side model picker needs.
type Model struct {
	// ID is the provider-side model identifier — what a caller sends back as
	// ProviderConfig.Model.
	ID string
	// OwnedBy is whatever attribution the provider gives alongside the id
	// ("openai", "library" on ollama, a Modelfile's From owner, …). Providers
	// that omit it leave this empty.
	OwnedBy string
}

// ListModels validates cfg's endpoint and token by asking for its model
// catalog: GET /v1/models is the one call every OpenAI-compatible server
// (OpenAI, ollama, LM Studio, llama.cpp) implements, and it is also the
// lightest one, so it doubles as the config-validation call — there is no
// separate "ping" RPC. A successful response already proves the base URL is
// reachable and the token (if any) is accepted; the error sentinels below
// distinguish why it failed.
//
// cfg.Model is not required and not used — a listing call by definition
// precedes choosing one.
//
// This is a single attempt, not run through the retry policy CompleteJSON
// uses: it's a synchronous, user-facing "does this work" check, so a
// transient failure should surface immediately rather than hold the caller
// for the retry budget.
func ListModels(ctx context.Context, cfg Config, opts ...Option) ([]Model, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("llm client requires an endpoint")
	}
	api, _ := newAPIClient(cfg, opts...)
	resp, err := api.ListModels(ctx)
	if err != nil {
		mapped, _ := classify(err)
		return nil, mapped
	}
	models := make([]Model, 0, len(resp.Models))
	for _, m := range resp.Models {
		models = append(models, Model{ID: m.ID, OwnedBy: m.OwnedBy})
	}
	return models, nil
}

// nonChatModelIDPatterns are substrings of an OpenAI model id that mark its
// FAMILY as unusable as the `model` field of a Chat Completions request.
//
// Why a name heuristic at all: GET /v1/models carries no capability,
// modality, or type field to filter by. Verified directly — both against the
// live shape (id, object, created, owned_by, shutdown_date; see
// https://developers.openai.com/api/reference/resources/models/methods/list)
// and against the go-openai Model struct this client already deserializes
// into (id, object, created, owned_by, permission, root, parent — same
// story, no capability field either). OpenAI's own community forum has an
// open, unanswered feature request asking for exactly this:
// "Expose Model Capabilities in the /v1/models API Response"
// (community.openai.com/t/expose-model-capabilities-in-the-v1-models-api-response/1314117).
// So id substring matching is the only signal available.
//
// This is why the check below is a DENY-list, not an allow-list: a new chat
// model OpenAI ships tomorrow under a name this list has never seen must
// still come through. An allow-list would silently drop it.
//
// A narrower, separate question this list does NOT answer: whether a given
// chat model supports response_format:{type:"json_schema", strict:true}
// (what our importv2 planner requires). Structured Outputs support is not
// exposed by /v1/models either — it was the other example in the same
// unanswered forum request above — and it is not derivable from the id
// pattern the way "this is an embedding model" is (both structured-outputs
// and non-structured-outputs models share the same "gpt-*" naming). Chat
// Completions usability and "supports our strict-JSON planner call" are
// different, narrower questions; this filter only answers the first one.
var nonChatModelIDPatterns = []string{
	"embedding",  // text-embedding-3-small/large, text-embedding-ada-002
	"whisper",    // whisper-1: speech-to-text
	"transcribe", // gpt-4o-transcribe, gpt-4o-mini-transcribe: speech-to-text
	"tts",        // tts-1, tts-1-hd, gpt-4o-mini-tts: text-to-speech
	// gpt-4o-audio-preview, gpt-4o-mini-audio-preview: reachable via
	// /chat/completions with an audio modality, but that is a materially
	// different request shape than the plain text chat this filter targets
	"audio",
	"realtime",     // gpt-4o-realtime-preview: websocket Realtime API only, not /chat/completions
	"image",        // gpt-image-1
	"dall-e",       // dall-e-2, dall-e-3
	"moderation",   // omni-moderation-*, text-moderation-*
	"davinci",      // davinci-002: legacy /v1/completions, not /chat/completions
	"babbage",      // babbage-002: legacy /v1/completions
	"instruct",     // gpt-3.5-turbo-instruct: legacy /v1/completions despite the "gpt" name
	"computer-use", // computer-use-preview: Responses/Operator tool-use model, not chat
}

// FilterChatModels narrows a raw catalog down to models usable for chat
// completions. It only makes sense for OPENAI: see nonChatModelIDPatterns
// for why (no capability field, deny-list on the id). Every other supported
// provider (OLLAMA, LMSTUDIO, LLAMACPP) is local/self-hosted and its catalog
// already contains only what the user chose to pull or load onto that
// server — nothing to filter out, so callers should pass those lists through
// unfiltered rather than run them through this function.
func FilterChatModels(models []Model) []Model {
	out := make([]Model, 0, len(models))
	for _, m := range models {
		if isChatCompletionModel(m.ID) {
			out = append(out, m)
		}
	}
	return out
}

func isChatCompletionModel(id string) bool {
	lower := strings.ToLower(id)
	for _, pattern := range nonChatModelIDPatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}
	return true
}
