package tasks

import (
	"context"

	"github.com/pgvector/pgvector-go"
	"github.com/sashabaranov/go-openai"
)

var TypeGenerateEmbedding = "task:generate_embedding"

func GenerateEmbedding(text string, apiKey string) (pgvector.Vector, error) {
	// 1. Rough estimate: 1 token is ~4 characters.
	// Max 8192 tokens * 4 = ~32,000 characters.
	maxChars := 10000
	if len(text) > maxChars {
		text = text[:maxChars]
	}

	client := openai.NewClient(apiKey)

	resp, err := client.CreateEmbeddings(
		context.Background(),
		openai.EmbeddingRequest{
			Input: []string{text},
			Model: openai.AdaEmbeddingV2, // Or text-embedding-3-small
		},
	)
	if err != nil {
		return pgvector.Vector{}, err
	}

	return pgvector.NewVector(resp.Data[0].Embedding), nil
}
