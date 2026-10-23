package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func GetQuotaBytesForPlan(tx pgx.Tx, ctx context.Context, planCode string) (int64, int64, error) {
	var planID int64
	var quotaBytes int64
	err := tx.QueryRow(ctx, `
		SELECT id, storage_limit_bytes
		FROM plans
		WHERE code = $1 AND is_active = TRUE
	`, planCode).Scan(&planID, &quotaBytes)
	if err != nil {
		return 0, 0, err
	}
	return planID, quotaBytes, nil
}
