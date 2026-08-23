package sagemaker

import (
	"github.com/takara-ai/miru-code/internal/embed"
)

// Re-export embed SageMaker helpers for callers that prefer this package path.

type Config = embed.SageMakerConfig

func Enabled() bool { return embed.IsSageMakerConfigured() }

func ResolveConfig() (*Config, error) { return embed.ResolveSageMakerConfig() }

func ParseEndpointARN(arn string) (region, accountID, endpointName string, err error) {
	return embed.ParseSageMakerEndpointARN(arn)
}

func NewBackend(cfg Config) embed.EmbeddingBackend {
	return embed.NewOpenAIEmbeddingBackend(&embed.OpenAIOptions{
		Model:      embed.SageMakerModelID(cfg.EndpointName),
		Dimensions: embed.ResolveEmbeddingDimensions(embed.SageMakerModelID(cfg.EndpointName)),
		Client:     embed.NewSageMakerClient(cfg),
	})
}

func ValidateConnection(cfg Config) (valid bool, status int, message string) {
	return embed.ValidateSageMakerConnection(cfg)
}
