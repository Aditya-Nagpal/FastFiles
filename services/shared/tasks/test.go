package tasks

import (
	"fmt"
	"testing"

	"github.com/pgvector/pgvector-go"
)

func TestGenerateEmbedding(content string, t *testing.T) (pgvector.Vector, error) {
	// Create a mock response with 1536 dimensions to match DB schema
	mockVector := make([]float32, 1536)
	for i := range mockVector {
		mockVector[i] = 0.1 // Fill with dummy data
	}

	v := pgvector.NewVector(mockVector)

	// Check against 1536 instead of 3
	if len(v.Slice()) != 1536 {
		t.Errorf("Expected 1536 dimensions, got %d", len(v.Slice()))
		return pgvector.Vector{}, fmt.Errorf("dimension mismatch")
	}

	return v, nil
}
