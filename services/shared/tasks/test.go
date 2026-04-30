package tasks

import (
	"fmt"

	"github.com/pgvector/pgvector-go"
)

func TestGenerateEmbedding(content string) (pgvector.Vector, error) {
	mockVector := make([]float32, 1536)
	for i := range mockVector {
		mockVector[i] = 0.1
	}

	v := pgvector.NewVector(mockVector)

	if len(v.Slice()) != 1536 {
		return pgvector.Vector{}, fmt.Errorf("expected 1536 dimensions, got %d", len(v.Slice()))
	}

	return v, nil
}
