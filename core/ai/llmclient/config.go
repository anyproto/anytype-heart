// Package llmclient is a thin, provider-agnostic client for OpenAI-compatible
// chat-completion endpoints (OpenAI, ollama, LM Studio, llama.cpp, …), built
// for structured-output calls: one non-streaming completion constrained by a
// strict JSON schema (docs/ImportV2LLM.md §6).
//
// The package is a plain library — no app component, no global state — so any
// feature needing a BYOK LLM call can share it. Determinism is part of the
// contract: temperature is forced to zero regardless of configuration.
package llmclient

import (
	"fmt"
	"net/url"
	"time"

	"github.com/anyproto/anytype-heart/pb"
)

// Config identifies one OpenAI-compatible endpoint and model.
type Config struct {
	// Endpoint is the base URL including the /v1 prefix, e.g.
	// "https://api.openai.com/v1" or "http://localhost:11434/v1".
	Endpoint string
	// Model is the provider-side model identifier ("gpt-4o", "qwen3:8b", …).
	Model string
	// Token is the bearer token. Local servers ignore the value but some
	// (ollama) still require the header; an empty token sends none.
	Token string
}

// Default endpoints per provider, used when the request leaves Endpoint empty.
const (
	defaultEndpointOpenAI   = "https://api.openai.com/v1"
	defaultEndpointOllama   = "http://localhost:11434/v1"
	defaultEndpointLMStudio = "http://localhost:1234/v1"
	defaultEndpointLlamaCpp = "http://localhost:8080/v1"
)

// FromProto normalizes a wire ProviderConfig into a Config, filling the
// provider's conventional endpoint when none is given. It returns ok=false
// when the config is absent or names no model — the callers' "feature off"
// signal — and an error only for configs that are present but unusable.
func FromProto(cfg *pb.RpcAIProviderConfig) (Config, bool, error) {
	if cfg == nil || cfg.Model == "" {
		return Config{}, false, nil
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		switch cfg.Provider {
		case pb.RpcAI_OPENAI:
			endpoint = defaultEndpointOpenAI
		case pb.RpcAI_OLLAMA:
			endpoint = defaultEndpointOllama
		case pb.RpcAI_LMSTUDIO:
			endpoint = defaultEndpointLMStudio
		case pb.RpcAI_LLAMACPP:
			endpoint = defaultEndpointLlamaCpp
		default:
			return Config{}, false, fmt.Errorf("unknown provider %v and no endpoint given", cfg.Provider)
		}
	}
	if cfg.Provider == pb.RpcAI_OPENAI {
		if cfg.Token == "" {
			return Config{}, false, fmt.Errorf("provider openai requires an api token")
		}
		// Local-server tokens are usually dummies; an OpenAI key is real.
		if err := checkTokenTransport(endpoint, cfg.Token); err != nil {
			return Config{}, false, err
		}
	}
	return Config{Endpoint: endpoint, Model: cfg.Model, Token: cfg.Token}, true, nil
}

// checkTokenTransport refuses to send a bearer token in cleartext to a
// non-local endpoint — a BYOK key must not leak over plain http.
func checkTokenTransport(endpoint, token string) error {
	if token == "" {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "http" {
		return nil
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return fmt.Errorf("refusing to send the api token over plain http to %q; use https or a local endpoint", host)
}

// RetryPolicy bounds the client's retries of transient failures (429, 5xx,
// transport errors). Modeled on the importv2 notion client's policy.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first
	BaseDelay   time.Duration // first backoff; doubles per attempt
	MaxDelay    time.Duration // backoff cap
}

// DefaultRetryPolicy suits an interactive import: a few quick retries, never
// minutes — the caller's wall-clock budget (ctx deadline) has the last word.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: 15 * time.Second}
}
