package db

import (
	"context"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/db"
	"github.com/pgvector/pgvector-go"
)

func InsertChunkVector(ctx context.Context, entryId int64, index int, content string, vector pgvector.Vector) error {
	query := `INSERT INTO file_chunks (entry_id, chunk_index, content, embedding) VALUES ($1, $2, $3, $4)`
	_, err := db.DB.Exec(ctx, query, entryId, index, content, vector)
	if err != nil {
		return err
	}
	return nil
}
