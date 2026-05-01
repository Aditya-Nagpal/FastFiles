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
			Model: openai.EmbeddingModel("text-embedding-3-small"), // Or text-embedding-3-small
		},
	)
	if err != nil {
		return pgvector.Vector{}, err
	}

	return pgvector.NewVector(resp.Data[0].Embedding), nil
}
