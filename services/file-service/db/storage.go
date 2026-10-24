package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/jackc/pgx/v5"
)

// ErrInsufficientStorage means the user has no available quota,
// or their storage record has not been initialized.
var ErrInsufficientStorage = errors.New("insufficient storage quota")

func ReserveStorage(ctx context.Context, userID int64, sizeBytes int64) error {
	if sizeBytes < 0 {
		return fmt.Errorf("file size cannot be negative")
	}

	if sizeBytes == 0 {
		return nil
	}

	query := `
		UPDATE user_storage
		SET reserved_bytes = reserved_bytes + $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
		  AND used_bytes + reserved_bytes + $1 <= quota_bytes
		RETURNING user_id
	`

	var reservedUserID int64
	err := DB.QueryRow(ctx, query, sizeBytes, userID).Scan(&reservedUserID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInsufficientStorage
	}
	if err != nil {
		return fmt.Errorf("failed to reserve storage: %w", err)
	}

	return nil
}

func ReleaseStorageReservation(ctx context.Context, userID int64, sizeBytes int64) error {
	if sizeBytes < 0 {
		return fmt.Errorf("file size cannot be negative")
	}

	if sizeBytes == 0 {
		return nil
	}

	query := `
		UPDATE user_storage
		SET reserved_bytes = reserved_bytes - $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
		  AND reserved_bytes >= $1
		RETURNING user_id
	`

	var id int64
	err := DB.QueryRow(ctx, query, sizeBytes, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("reservation not found or insufficient reserved bytes")
	}
	if err != nil {
		return fmt.Errorf("failed to release storage reservation: %w", err)
	}

	return nil
}

func ReleaseStorageReservationUsingTransaction(tx pgx.Tx, ctx context.Context, data *models.EntryData) (int64, error) {
	query := `
		UPDATE user_storage
		SET reserved_bytes = reserved_bytes - $1,
			used_bytes = used_bytes + $1,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
			AND reserved_bytes >= $1
		RETURNING user_id
	`

	var id int64
	err := tx.QueryRow(ctx, query, data.Size, data.UserId).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to finalize storage reservation: %w", err)
	}
	return id, nil
}
