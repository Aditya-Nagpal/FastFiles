package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func InitializeUserSubscription(tx pgx.Tx, ctx context.Context, userID int64, planID int64, quotaBytes int64) error {
	now := time.Now().UTC()

	_, err := tx.Exec(ctx, `
		INSERT INTO user_subscriptions (
			user_id,
			plan_id,
			status,
			started_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, 'ACTIVE', $3, $3, $3)
	`, userID, planID, now)
	if err != nil {
		return err
	}
	return nil
}
