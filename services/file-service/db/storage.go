package db

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Returns the reservation ID.
func CreateStorageReservation(ctx context.Context, userID int64, sizeBytes int64) (int64, error) {
	if sizeBytes < 0 {
		return 0, fmt.Errorf("file size cannot be negative")
	}

	tx, err := DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin reservation transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Reserve quota.
	query := `
        UPDATE user_storage
        SET reserved_bytes = reserved_bytes + $1,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = $2
          AND used_bytes + reserved_bytes + $1 <= quota_bytes
        RETURNING user_id
    `

	var reservedUserID int64

	err = tx.QueryRow(ctx, query, sizeBytes, userID).Scan(&reservedUserID)

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInsufficientStorage
	}

	if err != nil {
		return 0, fmt.Errorf("failed to reserve storage: %w", err)
	}

	// 2. Create an individual reservation record.
	// Initial lease duration: 5 minutes.
	insertQuery := `
        INSERT INTO storage_reservations (
            user_id,
            size_bytes,
            status,
            lease_expires_at
        )
        VALUES (
            $1,
            $2,
            'PENDING',
            CURRENT_TIMESTAMP + INTERVAL '5 minutes'
        )
        RETURNING id
    `

	var reservationID int64

	err = tx.QueryRow(
		ctx,
		insertQuery,
		userID,
		sizeBytes,
	).Scan(&reservationID)

	if err != nil {
		return 0, fmt.Errorf("failed to create storage reservation record: %w", err)
	}

	// 3. Commit both operations together.
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit storage reservation: %w", err)
	}

	return reservationID, nil
}

// RenewStorageReservation extends the lease of a pending reservation.
func RenewStorageReservation(
	ctx context.Context,
	reservationID int64,
	leaseDuration time.Duration,
) error {
	if leaseDuration <= 0 {
		return fmt.Errorf("lease duration must be positive")
	}

	query := `
        UPDATE storage_reservations
        SET lease_expires_at = CURRENT_TIMESTAMP
            + ($1 * INTERVAL '1 second'),
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $2
          AND status = 'PENDING'
    `

	tag, err := DB.Exec(
		ctx,
		query,
		leaseDuration.Seconds(),
		reservationID,
	)

	if err != nil {
		return fmt.Errorf(
			"failed to renew reservation %d: %w",
			reservationID,
			err,
		)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"reservation %d does not exist or is no longer pending",
			reservationID,
		)
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

// It is safe to retry: if the reservation has already been released,
// the function returns nil without subtracting quota again.
func ReleaseStorageReservationByID(ctx context.Context, reservationID int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin reservation release transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Lock and fetch the reservation.
	var (
		userID    int64
		sizeBytes int64
		status    string
	)

	err = tx.QueryRow(ctx, `
        SELECT user_id, size_bytes, status
        FROM storage_reservations
        WHERE id = $1
        FOR UPDATE
    `, reservationID).Scan(&userID, &sizeBytes, &status)

	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("storage reservation %d not found", reservationID)
	}

	if err != nil {
		return fmt.Errorf("failed to fetch storage reservation: %w", err)
	}

	// 2. Already released: safe to return.
	if status == "RELEASED" {
		return tx.Commit(ctx)
	}

	// 3. Never release a finalized reservation.
	if status != "PENDING" {
		return fmt.Errorf(
			"cannot release reservation %d with status %s",
			reservationID,
			status,
		)
	}

	// 4. Decrease reserved bytes.
	tag, err := tx.Exec(ctx, `
        UPDATE user_storage
        SET reserved_bytes = reserved_bytes - $1,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = $2
          AND reserved_bytes >= $1
    `, sizeBytes, userID)

	if err != nil {
		return fmt.Errorf("failed to decrease reserved storage: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("storage accounting inconsistency for user %d", userID)
	}

	// 5. Mark this reservation as released.
	_, err = tx.Exec(ctx, `
        UPDATE storage_reservations
        SET status = 'RELEASED',
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
          AND status = 'PENDING'
    `, reservationID)

	if err != nil {
		return fmt.Errorf("failed to update reservation status: %w", err)
	}

	// 6. Commit both changes together.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit reservation release: %w", err)
	}

	return nil
}

func FinalizeStorageReservationUsingTransaction(
	tx pgx.Tx,
	ctx context.Context,
	reservationID int64,
	userID int64,
	sizeBytes int64,
) error {
	// 1. Lock and validate the reservation.
	var (
		reservationUserID int64
		reservationSize   int64
		status            string
	)

	err := tx.QueryRow(ctx, `
        SELECT user_id, size_bytes, status
        FROM storage_reservations
        WHERE id = $1
        FOR UPDATE
    `, reservationID).Scan(
		&reservationUserID,
		&reservationSize,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("storage reservation %d not found", reservationID)
	}

	if err != nil {
		return fmt.Errorf("failed to fetch reservation: %w", err)
	}

	if status != "PENDING" {
		return fmt.Errorf(
			"cannot finalize reservation %d: status is %s",
			reservationID,
			status,
		)
	}

	if reservationUserID != userID {
		return fmt.Errorf("reservation does not belong to user %d", userID)
	}

	if reservationSize != sizeBytes {
		return fmt.Errorf(
			"reservation size mismatch: reserved=%d, upload=%d",
			reservationSize,
			sizeBytes,
		)
	}

	// 2. Move reserved bytes to used bytes.
	if sizeBytes > 0 {
		tag, err := tx.Exec(ctx, `
            UPDATE user_storage
            SET reserved_bytes = reserved_bytes - $1,
                used_bytes = used_bytes + $1,
                updated_at = CURRENT_TIMESTAMP
            WHERE user_id = $2
              AND reserved_bytes >= $1
        `, sizeBytes, userID)

		if err != nil {
			return fmt.Errorf("failed to update storage accounting: %w", err)
		}

		if tag.RowsAffected() != 1 {
			return fmt.Errorf("storage accounting inconsistency for user %d", userID)
		}
	}

	// 3. Mark reservation as finalized.
	tag, err := tx.Exec(ctx, `
        UPDATE storage_reservations
        SET status = 'FINALIZED',
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
          AND status = 'PENDING'
    `, reservationID)

	if err != nil {
		return fmt.Errorf("failed to finalize reservation record: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf("failed to mark reservation %d as finalized", reservationID)
	}

	return nil
}

// RecoverExpiredStorageReservations releases expired pending reservations.
//
// FOR UPDATE SKIP LOCKED allows multiple recovery workers to process
// different reservations concurrently without processing the same row.
func RecoverExpiredStorageReservations(
	ctx context.Context,
	batchSize int,
) (int, error) {
	if batchSize <= 0 {
		return 0, fmt.Errorf("batch size must be positive")
	}

	tx, err := DB.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin recovery transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Lock a batch of expired pending reservations.
	rows, err := tx.Query(ctx, `
        SELECT id, user_id, size_bytes
        FROM storage_reservations
        WHERE status = 'PENDING'
          AND lease_expires_at <= CURRENT_TIMESTAMP
        ORDER BY lease_expires_at ASC
        LIMIT $1
        FOR UPDATE SKIP LOCKED
    `, batchSize)

	if err != nil {
		return 0, fmt.Errorf("failed to fetch expired reservations: %w", err)
	}

	type expiredReservation struct {
		id        int64
		userID    int64
		sizeBytes int64
	}

	var reservations []expiredReservation

	for rows.Next() {
		var reservation expiredReservation

		if err := rows.Scan(
			&reservation.id,
			&reservation.userID,
			&reservation.sizeBytes,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("failed to scan reservation: %w", err)
		}

		reservations = append(reservations, reservation)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("failed to read expired reservations: %w", err)
	}

	rows.Close()

	// 2. Release each reservation and update its status.
	for _, reservation := range reservations {
		tag, err := tx.Exec(ctx, `
            UPDATE user_storage
            SET reserved_bytes = reserved_bytes - $1,
                updated_at = CURRENT_TIMESTAMP
            WHERE user_id = $2
              AND reserved_bytes >= $1
        `, reservation.sizeBytes, reservation.userID)

		if err != nil {
			return 0, fmt.Errorf(
				"failed to release quota for reservation %d: %w",
				reservation.id,
				err,
			)
		}

		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf(
				"storage accounting inconsistency for reservation %d",
				reservation.id,
			)
		}

		tag, err = tx.Exec(ctx, `
            UPDATE storage_reservations
            SET status = 'RELEASED',
                updated_at = CURRENT_TIMESTAMP
            WHERE id = $1
              AND status = 'PENDING'
        `, reservation.id)

		if err != nil {
			return 0, fmt.Errorf(
				"failed to mark reservation %d as released: %w",
				reservation.id,
				err,
			)
		}

		if tag.RowsAffected() != 1 {
			return 0, fmt.Errorf(
				"failed to update reservation %d status",
				reservation.id,
			)
		}
	}

	// 3. Commit the entire batch atomically.
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit reservation recovery: %w", err)
	}

	return len(reservations), nil
}

func ReleaseQuota(tx pgx.Tx, ctx context.Context, userID int64, sizeBytes int64) (int64, error) {
	query := `
		UPDATE user_storage
		SET used_bytes = used_bytes - $1,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
			AND used_bytes >= $1
		RETURNING user_id
	`

	var updatedUserID int64
	err := tx.QueryRow(ctx, query, sizeBytes, userID).Scan(&updatedUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("storage accounting inconsistency: cannot release %d bytes for user %d", sizeBytes, userID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to release quota: %w", err)
	}
	return updatedUserID, nil
}
