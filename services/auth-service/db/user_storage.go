package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func InitializeUserStorage(tx pgx.Tx, ctx context.Context, userID int64, quotaBytes int64) error {
	now := time.Now().UTC()

	_, err := tx.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes,
			updated_at
		)
		VALUES ($1, $2, 0, 0, $3)
	`, userID, quotaBytes, now)
	if err != nil {
		return err
	}
	return nil
}
