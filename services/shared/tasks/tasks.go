package tasks

import (
	"context"

	"github.com/pgvector/pgvector-go"
	"github.com/sashabaranov/go-openai"
)

var TypeGenerateEmbedding = "task:generate_embedding"

func GenerateEmbedding(text string, apiKey string) (pgvector.Vector, error) {
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

	// Convert float32 slice to pgvector.Vector
	return pgvector.NewVector(resp.Data[0].Embedding), nil
}
