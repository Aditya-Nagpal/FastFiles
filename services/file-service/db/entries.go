package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

func GetInternalID(ctx context.Context, publicId string, userId int64) (*int64, error) {
	query := `SELECT id FROM entries WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL`

	var internalId int64
	err := DB.QueryRow(ctx, query, publicId, userId).Scan(&internalId)
	if err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return &internalId, nil
}

func GetEntityType(ctx context.Context, publicId string, userId int64) (string, error) {
	query := `SELECT type FROM entries WHERE public_id = $1 AND user_id = $2 AND deleted_at IS NULL`

	var entityType string
	err := DB.QueryRow(ctx, query, publicId, userId).Scan(&entityType)
	if err == pgx.ErrNoRows {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return strings.ToLower(entityType), nil
}

func GetFilesByParentId(ctx context.Context, userId int64, internalParentID *int64) ([]models.ListFileResponse, error) {
	query := `
		SELECT
			public_id,
			name,
			type,
			content_type,
			extension,
			size,
			created_at,
			updated_at
		FROM entries
		WHERE user_id = $1
			AND (
				($2::BIGINT IS NULL AND parent_id IS NULL)
				OR
				(parent_id = $2)
			)
			AND deleted_at IS NULL
		ORDER BY
			type DESC,
			updated_at DESC
	`
	rows, err := DB.Query(ctx, query, userId, internalParentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]models.ListFileResponse, 0)
	for rows.Next() {
		var file models.ListFileResponse
		if err := rows.Scan(&file.PublicId, &file.Name, &file.Type, &file.ContentType, &file.Extension, &file.Size, &file.CreatedAt, &file.UpdatedAt); err != nil {
			return nil, err
		}

		if file.Extension != "" {
			file.Name = file.Name + "." + file.Extension
		}

		files = append(files, file)
	}

	return files, nil
}

func InsertEntryData(ctx context.Context, data *models.EntryData) (int64, error) {
	query := `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, content_type, extension, size, s3_key, created_at, updated_at)
	    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id
	`

	var newId int64
	err := DB.QueryRow(ctx, query, data.PublicId, data.UserId, data.ParentId, data.Name, data.Type, data.ContentType, data.Extension, data.Size, data.S3Key, data.CreatedAt, data.UpdatedAt).Scan(&newId)
	if err != nil {
		return 0, err
	}
	return newId, nil
}

func InsertEntryAndFinalizeStorage(ctx context.Context, data *models.EntryData) (int64, error) {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// 1. Convert reserved bytes into used bytes.
	if data.Size > 0 {
		_, err = ReleaseStorageReservationUsingTransaction(tx, ctx, data)
		if err != nil {
			return 0, fmt.Errorf("failed to finalize storage reservation: %w", err)
		}
	}

	// 2. Insert the entry.
	query := `
		INSERT INTO entries (
			public_id, user_id, parent_id, name, type,
			content_type, extension, size, s3_key,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`

	var newID int64
	err = tx.QueryRow(
		ctx,
		query,
		data.PublicId,
		data.UserId,
		data.ParentId,
		data.Name,
		data.Type,
		data.ContentType,
		data.Extension,
		data.Size,
		data.S3Key,
		data.CreatedAt,
		data.UpdatedAt,
	).Scan(&newID)
	if err != nil {
		return 0, err
	}

	// 3. Insert the outbox event.
	// We only publish FILE_CREATED for files, not folders.
	eventID := uuid.NewString()
	if data.Type == "FILE" {
		eventPayload, err := json.Marshal(map[string]any{
			"eventId":     eventID,
			"eventType":   "FILE_CREATED",
			"aggregateId": data.PublicId,
			"userId":      data.UserId,
			"entryId":     newID,
			"publicId":    data.PublicId,
			"parentId":    data.ParentId,
			"name":        data.Name,
			"entryType":   data.Type,
			"size":        data.Size,
			"s3Key":       data.S3Key.String,
			"createdAt":   data.CreatedAt.UTC(),
		})
		if err != nil {
			return 0, fmt.Errorf("failed to marshal outbox event: %w", err)
		}

		err = InsertOutboxEvent(tx, ctx, eventID, eventPayload, "ENTRY", data.PublicId, "FILE_CREATED")
		if err != nil {
			return 0, fmt.Errorf("failed to insert outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit entry transaction: %w", err)
	}

	return newID, nil
}

var ErrEntryNotFound = errors.New("entry not found")

func DeleteFile(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Soft-delete the active file and obtain its actual size.
	var (
		entryID  int64
		parentID sql.NullInt64
		name     string
		fileSize int64
	)

	query := `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE public_id = $1 AND user_id = $2 AND type = 'FILE' AND deleted_at IS NULL
		RETURNING id, parent_id, name, size
	`

	err = tx.QueryRow(ctx, query, publicId, userId).Scan(&entryID, &parentID, &name, &fileSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to soft-delete file: %w", err)
	}

	// Release exactly the file's size from committed usage.
	if fileSize > 0 {
		_, err := ReleaseQuota(tx, ctx, userId, fileSize)
		if err != nil {
			return fmt.Errorf("failed to release quota: %w", err)
		}
	}

	// Event payload for the outbox event.
	var parentPtr *int64
	if parentID.Valid {
		parentPtr = &parentID.Int64
	}

	eventID := uuid.NewString()
	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFileDeleted,
		AggregateID: publicId,
		UserID:      userId,
		EntryID:     entryID,
		PublicID:    publicId,
		ParentID:    parentPtr,
		Name:        name,
		EntryType:   "FILE",
		Size:        fileSize,
		CreatedAt:   time.Now().UTC(),
	})

	if err := InsertOutboxEvent(tx, ctx, eventID, payload, "ENTRY", publicId, events.EventFileDeleted); err != nil {
		return err
	}

	// Soft delete, quota release and event commit together.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit file deletion: %w", err)
	}

	return nil
}

func DeleteFolder(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock the root folder so concurrent deletion attempts
	// cannot both delete and release quota for it.
	var folderID int64

	err = tx.QueryRow(ctx, `
		SELECT id
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND type = 'FOLDER'
		  AND deleted_at IS NULL
		FOR UPDATE
	`, publicId, userId).Scan(&folderID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find folder: %w", err)
	}

	// Calculate the actual size of all active files in this subtree.
	var totalSize int64

	err = tx.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, type, size
			FROM entries
			WHERE id = $1
			  AND user_id = $2
			  AND deleted_at IS NULL

			UNION ALL

			SELECT e.id, e.parent_id, e.type, e.size
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
			  AND e.deleted_at IS NULL
		)
		SELECT COALESCE(SUM(size) FILTER (WHERE type = 'FILE'), 0)
		FROM descendants
	`, folderID, userId).Scan(&totalSize)

	if err != nil {
		return fmt.Errorf("failed to calculate folder size: %w", err)
	}

	// Soft-delete the root and all active descendants.
	_, err = tx.Exec(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM entries
			WHERE id = $1
			  AND user_id = $2
			  AND deleted_at IS NULL

			UNION ALL

			SELECT e.id
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
			  AND e.deleted_at IS NULL
		)
		UPDATE entries
		SET deleted_at = $3,
		    updated_at = $3
		WHERE id IN (SELECT id FROM descendants)
		  AND user_id = $2
		  AND deleted_at IS NULL
	`, folderID, userId, time.Now().UTC())

	if err != nil {
		return fmt.Errorf("failed to soft-delete folder contents: %w", err)
	}

	// Release the total actual file size, not the cached folder size.
	if totalSize > 0 {
		_, err := ReleaseQuota(tx, ctx, userId, totalSize)
		if err != nil {
			return fmt.Errorf("failed to release quota: %w", err)
		}
	}

	eventID := uuid.NewString()
	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFolderDeleted,
		AggregateID: publicId,
		UserID:      userId,
		EntryID:     folderID,
		PublicID:    publicId,
		EntryType:   "FOLDER",
		Size:        totalSize,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal folder deletion event: %w", err)
	}

	if err := InsertOutboxEvent(tx, ctx, eventID, payload, "ENTRY", publicId, events.EventFolderDeleted); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit folder deletion: %w", err)
	}

	return nil
}

func GetDownloadFile(ctx context.Context, publicId string, userId int64) (*models.DownloadFile, error) {
	query := `SELECT name, extension, type, s3_key FROM entries WHERE public_id = $1 AND user_id = $2`

	var file models.DownloadFile

	err := DB.QueryRow(ctx, query, publicId, userId).Scan(&file.Name, &file.Extension, &file.Type, &file.S3Key)
	if err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &file, nil
}

func SearchByVector(ctx context.Context, vector pgvector.Vector, limit int, userId int64) ([]models.SearchResponse, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT * FROM (
			SELECT DISTINCT ON (e.id)
				e.id, e.public_id, e.name, e.type,
				1 - (fc.embedding <=> $1) AS score
			FROM entries e
			JOIN file_chunks fc ON e.id = fc.entry_id
			WHERE e.user_id = $2 AND e.deleted_at IS NULL AND e.type = 'FILE'
		) AS subquery
		ORDER BY score DESC
		LIMIT $3
	`

	rows, err := DB.Query(ctx, query, vector, userId, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]models.SearchResponse, 0)
	for rows.Next() {
		var file models.SearchResponse
		if err := rows.Scan(&file.ID, &file.PublicID, &file.Name, &file.Type, &file.Score); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
