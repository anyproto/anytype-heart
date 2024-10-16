package ai

import (
	"fmt"
	"strings"
)

func switchModelProvider(provider string, config *APIConfig) {
	switch strings.ToLower(provider) {
	case "ollama":
		config.Provider = ProviderOllama
		config.Endpoint = ollamaEndpointChat
		config.EndpointModels = ollamaEndpointModels
		config.Model = ollamaDefaultModelChat
		config.AuthRequired = false
		fmt.Println("Switched to Ollama API")

	case "openai":
		config.Provider = ProviderOpenAI
		config.Endpoint = openaiEndpointChat
		config.EndpointModels = openaiEndpointModels
		config.Model = openaiDefaultModelChat
		config.AuthRequired = true
		config.AuthToken = openaiAPIKey
		fmt.Println("Switched to OpenAI API")

	default:
		fmt.Println("Unknown provider:", provider)
	}

	// Debug
	fmt.Println("Current chat config:")
	fmt.Printf("Provider: %v\n", config.Provider)
	fmt.Printf("Endpoint: %v\n", config.Endpoint)
	fmt.Printf("EndpointModels: %v\n", config.EndpointModels)
	fmt.Printf("Model: %v\n", config.Model)
	fmt.Printf("AuthRequired: %v\n", config.AuthRequired)
	if config.AuthRequired {
		fmt.Printf("AuthToken: %s%s\n", config.AuthToken[0:10], strings.Repeat("*", len(config.AuthToken)-10))
	}
}

func switchEmbedProvider(provider string, config *APIConfig) {
	switch strings.ToLower(provider) {
	case "ollama":
		config.Provider = ProviderOllama
		config.Endpoint = ollamaEndpointEmbed
		config.EndpointModels = ollamaEndpointModels
		config.Model = ollamaDefaultModelEmbed
		config.AuthRequired = false
		fmt.Println("Switched to Ollama API")

	case "openai":
		config.Provider = ProviderOpenAI
		config.Endpoint = openaiEndpointEmbed
		config.EndpointModels = openaiEndpointModels
		config.Model = openaiDefaultModelEmbed
		config.AuthRequired = true
		config.AuthToken = openaiAPIKey
		fmt.Println("Switched to OpenAI API")

	default:
		fmt.Println("Unknown provider:", provider)
	}

	// Debug
	fmt.Println("Current embed config:")
	fmt.Printf("Provider: %v\n", config.Provider)
	fmt.Printf("Endpoint: %v\n", config.Endpoint)
	fmt.Printf("EndpointModels: %v\n", config.EndpointModels)
	fmt.Printf("Model: %v\n", config.Model)
	fmt.Printf("AuthRequired: %v\n", config.AuthRequired)
	if config.AuthRequired {
		fmt.Printf("AuthToken: %s%s\n", config.AuthToken[0:10], strings.Repeat("*", len(config.AuthToken)-10))
	}
}
