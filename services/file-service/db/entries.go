package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func GetTrashedEntityType(ctx context.Context, publicID string, userID int64) (string, error) {
	var entityType string

	err := DB.QueryRow(ctx, `
		SELECT type
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND deleted_at IS NOT NULL
	`, publicID, userID).Scan(&entityType)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEntryNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get trashed entity type: %w", err)
	}

	return entityType, nil
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
			updated_at,
			is_starred
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
		if err := rows.Scan(&file.PublicId, &file.Name, &file.Type, &file.ContentType, &file.Extension, &file.Size, &file.CreatedAt, &file.UpdatedAt, &file.IsStarred); err != nil {
			return nil, err
		}

		if file.Extension != "" {
			file.Name = file.Name + "." + file.Extension
		}

		files = append(files, file)
	}

	return files, nil
}

func InsertEntryData(ctx context.Context, data *models.EntryData) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock and validate the destination folder hierarchy.
	if data.ParentId != nil {
		err = LockEntryHierarchy(tx, ctx, data.UserId, *data.ParentId, "FOLDER", false)
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, content_type, extension, size, s3_key, created_at, updated_at)
	    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = tx.Exec(
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
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func InsertEntryAndFinalizeStorage(ctx context.Context, data *models.EntryData, reservationID int64) (int64, error) {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// 1. Lock and validate the destination folder hierarchy.
	if data.ParentId != nil {
		err = LockEntryHierarchy(tx, ctx, data.UserId, *data.ParentId, "FOLDER", false)
		if err != nil {
			return 0, fmt.Errorf("failed to lock destination hierarchy: %w", err)
		}
	}

	// 2. Finalize the specific reservation.
	err = FinalizeStorageReservationUsingTransaction(tx, ctx, reservationID, data.UserId, data.Size)
	if err != nil {
		return 0, fmt.Errorf("failed to finalize storage reservation: %w", err)
	}

	// 3. Insert the entry.
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

	// 4. Insert the outbox event.
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

var (
	ErrEntryNotFound         = errors.New("entry not found")
	ErrInvalidEntryHierarchy = errors.New("invalid entry hierarchy")
)

func LockEntryHierarchy(
	tx pgx.Tx,
	ctx context.Context,
	userID int64,
	entryID int64,
	expectedTargetType string,
	lockTargetForUpdate bool,
) error {
	// Read the target and its ancestors, ordered root -> target.
	rows, err := tx.Query(ctx, `
        WITH RECURSIVE ancestors AS (
            SELECT
                id,
                user_id,
                parent_id,
                type,
                deleted_at,
                ARRAY[id]::BIGINT[] AS path
            FROM entries
            WHERE id = $1 AND user_id = $2

            UNION ALL

            SELECT
                p.id,
                p.user_id,
                p.parent_id,
                p.type,
                p.deleted_at,
                a.path || p.id
            FROM entries p
            JOIN ancestors a ON a.parent_id = p.id
            WHERE p.user_id = $2
              AND NOT p.id = ANY(a.path)
        )
        SELECT id, user_id, parent_id, type, deleted_at
        FROM ancestors
        ORDER BY cardinality(path) DESC
    `, entryID, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type hierarchyEntry struct {
		id        int64
		userID    int64
		parentID  *int64
		entryType string
		deletedAt sql.NullTime
	}

	var hierarchy []hierarchyEntry

	for rows.Next() {
		var entry hierarchyEntry

		if err := rows.Scan(
			&entry.id,
			&entry.userID,
			&entry.parentID,
			&entry.entryType,
			&entry.deletedAt,
		); err != nil {
			return err
		}

		hierarchy = append(hierarchy, entry)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if len(hierarchy) == 0 {
		return ErrEntryNotFound
	}

	// The recursive query returns root -> target.
	target := hierarchy[len(hierarchy)-1]

	if target.id != entryID || target.entryType != expectedTargetType {
		return ErrInvalidEntryHierarchy
	}

	// Validate the hierarchy before acquiring locks.
	for i, entry := range hierarchy {
		if entry.deletedAt.Valid {
			return ErrEntryNotFound
		}

		if i == 0 {
			if entry.parentID != nil {
				return ErrInvalidEntryHierarchy
			}
			continue
		}

		parent := hierarchy[i-1]

		if entry.parentID == nil || *entry.parentID != parent.id {
			return ErrInvalidEntryHierarchy
		}

		if parent.entryType != "FOLDER" {
			return ErrInvalidEntryHierarchy
		}
	}

	// Lock ancestors first, then the target. This consistent order
	// helps avoid deadlocks between concurrent hierarchy mutations.
	for i, entry := range hierarchy {
		var lockedID int64

		if i == len(hierarchy)-1 && lockTargetForUpdate {
			err = tx.QueryRow(ctx, `
                SELECT id
                FROM entries
                WHERE id = $1
                  AND user_id = $2
                  AND deleted_at IS NULL
                FOR UPDATE
            `, entry.id, userID).Scan(&lockedID)
		} else {
			err = tx.QueryRow(ctx, `
                SELECT id
                FROM entries
                WHERE id = $1
                  AND user_id = $2
                  AND deleted_at IS NULL
                FOR SHARE
            `, entry.id, userID).Scan(&lockedID)
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEntryNotFound
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func LockTrashedEntryHierarchy(
	tx pgx.Tx,
	ctx context.Context,
	userID int64,
	entryID int64,
	expectedTargetType string,
) error {
	rows, err := tx.Query(ctx, `
        WITH RECURSIVE ancestors AS (
            SELECT
                id,
                user_id,
                parent_id,
                type,
                deleted_at,
                ARRAY[id]::BIGINT[] AS path
            FROM entries
            WHERE id = $1
              AND user_id = $2

            UNION ALL

            SELECT
                p.id,
                p.user_id,
                p.parent_id,
                p.type,
                p.deleted_at,
                a.path || p.id
            FROM entries p
            JOIN ancestors a ON a.parent_id = p.id
            WHERE p.user_id = $2
              AND NOT p.id = ANY(a.path)
        )
        SELECT id, parent_id, type, deleted_at
        FROM ancestors
        ORDER BY cardinality(path) DESC
    `, entryID, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type hierarchyEntry struct {
		id        int64
		parentID  *int64
		entryType string
		deletedAt sql.NullTime
	}

	var hierarchy []hierarchyEntry

	for rows.Next() {
		var entry hierarchyEntry

		if err := rows.Scan(
			&entry.id,
			&entry.parentID,
			&entry.entryType,
			&entry.deletedAt,
		); err != nil {
			return err
		}

		hierarchy = append(hierarchy, entry)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if len(hierarchy) == 0 {
		return ErrEntryNotFound
	}

	target := hierarchy[len(hierarchy)-1]

	if target.id != entryID || target.entryType != expectedTargetType {
		return ErrInvalidEntryHierarchy
	}

	// The target must be trashed, but all its ancestors must be active.
	for i, entry := range hierarchy {
		if i == len(hierarchy)-1 {
			if !entry.deletedAt.Valid {
				return ErrEntryNotFound
			}
			continue
		}

		if entry.deletedAt.Valid {
			return ErrEntryNotFound
		}

		if i == 0 {
			if entry.parentID != nil {
				return ErrInvalidEntryHierarchy
			}
			continue
		}

		parent := hierarchy[i-1]

		if entry.parentID == nil || *entry.parentID != parent.id {
			return ErrInvalidEntryHierarchy
		}

		if parent.entryType != "FOLDER" {
			return ErrInvalidEntryHierarchy
		}
	}

	// Lock active ancestors first, then the trashed target.
	for i, entry := range hierarchy {
		var lockedID int64

		if i == len(hierarchy)-1 {
			err = tx.QueryRow(ctx, `
                SELECT id
                FROM entries
                WHERE id = $1
                  AND user_id = $2
                  AND deleted_at IS NOT NULL
                FOR UPDATE
            `, entry.id, userID).Scan(&lockedID)
		} else {
			err = tx.QueryRow(ctx, `
                SELECT id
                FROM entries
                WHERE id = $1
                  AND user_id = $2
                  AND deleted_at IS NULL
                FOR SHARE
            `, entry.id, userID).Scan(&lockedID)
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEntryNotFound
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func SoftDeleteFile(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Find the active file before acquiring hierarchy locks.
	var (
		entryID  int64
		parentID sql.NullInt64
		name     string
		fileSize int64
	)

	err = tx.QueryRow(ctx, `
		SELECT id, parent_id, name, size
		FROM entries
		WHERE public_id = $1
			AND user_id = $2
			AND type = 'FILE'
			AND deleted_at IS NULL
	`, publicId, userId).Scan(&entryID, &parentID, &name, &fileSize)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find file: %w", err)
	}

	// Lock the parent hierarchy first, then lock the file itself.
	if parentID.Valid {
		err = LockEntryHierarchy(tx, ctx, userId, parentID.Int64, "FOLDER", false)
		if err != nil {
			return fmt.Errorf("failed to lock parent hierarchy: %w", err)
		}
	}

	err = LockEntryHierarchy(tx, ctx, userId, entryID, "FILE", true)
	if err != nil {
		return fmt.Errorf("failed to lock file hierarchy: %w", err)
	}

	// Move the file to Trash without releasing its storage quota.
	_, err = tx.Exec(ctx, `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP,
			is_starred = FALSE,
			explicitly_deleted = TRUE
		WHERE id = $1
			AND user_id = $2
			AND type = 'FILE'
			AND deleted_at IS NULL
	`, entryID, userId)

	if err != nil {
		return fmt.Errorf("failed to move file to trash: %w", err)
	}

	// Prepare parent ID for the event payload.
	var parentPtr *int64
	if parentID.Valid {
		parentPtr = &parentID.Int64
	}

	// Insert FILE_TRASHED outbox event.
	eventID := uuid.NewString()
	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFileTrashed,
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

	if err != nil {
		return fmt.Errorf("failed to marshal file trashed event: %w", err)
	}

	if err := InsertOutboxEvent(
		tx,
		ctx,
		eventID,
		payload,
		"ENTRY",
		publicId,
		events.EventFileTrashed,
	); err != nil {
		return fmt.Errorf("failed to insert file trashed event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit file trash operation: %w", err)
	}

	return nil
}

func SoftDeleteFolder(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Find the active folder first.
	var folderID int64

	err = tx.QueryRow(ctx, `
		SELECT id
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND type = 'FOLDER'
		  AND deleted_at IS NULL
	`, publicId, userId).Scan(&folderID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find folder: %w", err)
	}

	// Lock ancestors first, then exclusively lock the target folder.
	err = LockEntryHierarchy(tx, ctx, userId, folderID, "FOLDER", true)
	if err != nil {
		return fmt.Errorf("failed to lock folder hierarchy: %w", err)
	}

	now := time.Now().UTC()

	// Soft-delete the root folder and all currently active descendants.
	// Only the root is explicitly deleted. Existing flags on descendants
	// are preserved, including independently trashed descendants.
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
		    updated_at = $3,
		    is_starred = FALSE,
		    explicitly_deleted = CASE
		        WHEN id = $1 THEN TRUE
		        ELSE explicitly_deleted
		    END
		WHERE id IN (SELECT id FROM descendants)
		  AND user_id = $2
		  AND deleted_at IS NULL
	`, folderID, userId, now)

	if err != nil {
		return fmt.Errorf("failed to move folder contents to trash: %w", err)
	}

	// Insert FOLDER_TRASHED outbox event.
	eventID := uuid.NewString()
	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFolderTrashed,
		AggregateID: publicId,
		UserID:      userId,
		EntryID:     folderID,
		PublicID:    publicId,
		EntryType:   "FOLDER",
		CreatedAt:   now,
	})

	if err != nil {
		return fmt.Errorf("failed to marshal folder trashed event: %w", err)
	}

	if err := InsertOutboxEvent(
		tx,
		ctx,
		eventID,
		payload,
		"ENTRY",
		publicId,
		events.EventFolderTrashed,
	); err != nil {
		return fmt.Errorf("failed to insert folder trashed event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit folder trash operation: %w", err)
	}

	return nil
}

func GetDownloadFile(ctx context.Context, publicId string, userId int64) (*models.DownloadFile, error) {
	query := `SELECT name, extension, type, s3_key FROM entries WHERE public_id = $1 AND user_id = $2 AND type = $3`

	var file models.DownloadFile

	err := DB.QueryRow(ctx, query, publicId, userId, "FILE").Scan(&file.Name, &file.Extension, &file.Type, &file.S3Key)
	if err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &file, nil
}

func UpdateStarredStatus(ctx context.Context, userID int64, publicID string, isStarred bool) error {
	query := `
		UPDATE entries
		SET is_starred = $1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE public_id = $2
		  AND user_id = $3
		  AND deleted_at IS NULL
	`

	tag, err := DB.Exec(ctx, query, isStarred, publicID, userID)
	if err != nil {
		return fmt.Errorf("failed to update starred status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEntryNotFound
	}

	return nil
}

func GetStarredEntries(ctx context.Context, userID int64, limit int, offset int) ([]models.ListFileResponse, error) {
	query := `
		SELECT
			e.public_id,
			e.name,
			e.type,
			e.content_type,
			e.extension,
			e.size,
			e.created_at,
			e.updated_at,
			e.is_starred
		FROM entries e
		WHERE e.user_id = $1
		  AND e.is_starred = TRUE
		  AND e.deleted_at IS NULL
		ORDER BY e.updated_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := DB.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch starred entries: %w", err)
	}
	defer rows.Close()

	results := make([]models.ListFileResponse, 0)

	for rows.Next() {
		var item models.ListFileResponse

		err := rows.Scan(
			&item.PublicId,
			&item.Name,
			&item.Type,
			&item.ContentType,
			&item.Extension,
			&item.Size,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.IsStarred,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan starred entry: %w", err)
		}

		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read starred entries: %w", err)
	}

	return results, nil
}

func GetTrashEntries(ctx context.Context, userID int64, parentID *int64) ([]models.ListFileResponse, int, error) {
	var (
		query string
		args  []any
	)

	if parentID == nil {
		// Virtual Trash root: only explicitly trashed entries
		// that do not have an explicitly trashed ancestor.
		query = `
            WITH RECURSIVE ancestors AS (
                SELECT
                    e.id,
                    e.parent_id,
                    e.explicitly_deleted
                FROM entries e
                WHERE e.user_id = $1
                  AND e.deleted_at IS NOT NULL
                  AND e.explicitly_deleted = TRUE

                UNION ALL

                SELECT
                    child.id,
                    child.parent_id,
                    child.explicitly_deleted
                FROM entries child
                JOIN ancestors a ON child.id = a.parent_id
                WHERE child.user_id = $1
            )
            SELECT
                e.public_id,
                e.name,
                e.type,
                e.content_type,
                e.extension,
                e.size,
                e.created_at,
                e.updated_at,
                e.is_starred,
                COUNT(*) OVER() AS total
            FROM entries e
            WHERE e.user_id = $1
              AND e.deleted_at IS NOT NULL
              AND e.explicitly_deleted = TRUE
              AND NOT EXISTS (
                  WITH RECURSIVE parent_chain AS (
                      SELECT
                          p.id,
                          p.parent_id,
                          p.explicitly_deleted
                      FROM entries p
                      WHERE p.id = e.parent_id
                        AND p.user_id = $1

                      UNION ALL

                      SELECT
                          p.id,
                          p.parent_id,
                          p.explicitly_deleted
                      FROM entries p
                      JOIN parent_chain pc ON p.id = pc.parent_id
                      WHERE p.user_id = $1
                  )
                  SELECT 1
                  FROM parent_chain
                  WHERE explicitly_deleted = TRUE
              )
            ORDER BY e.type DESC, e.updated_at DESC
        `
		args = []any{userID}
	} else {
		// Contents of a trashed folder.
		query = `
            SELECT
                e.public_id,
                e.name,
                e.type,
                e.content_type,
                e.extension,
                e.size,
                e.created_at,
                e.updated_at,
                e.is_starred,
                COUNT(*) OVER() AS total
            FROM entries e
            WHERE e.user_id = $1
              AND e.parent_id = $2
              AND e.deleted_at IS NOT NULL
            ORDER BY e.type DESC, e.updated_at DESC
        `
		args = []any{userID, *parentID}
	}

	rows, err := DB.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	results := make([]models.ListFileResponse, 0)
	total := 0

	for rows.Next() {
		var item models.ListFileResponse

		if err := rows.Scan(
			&item.PublicId,
			&item.Name,
			&item.Type,
			&item.ContentType,
			&item.Extension,
			&item.Size,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.IsStarred,
			&total,
		); err != nil {
			return nil, 0, err
		}

		if item.Extension != "" {
			item.Name += "." + item.Extension
		}

		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

func GetTrashedFolderInternalID(ctx context.Context, publicID string, userID int64) (*int64, error) {
	query := `
        SELECT id
        FROM entries
        WHERE public_id = $1
          AND user_id = $2
          AND type = 'FOLDER'
          AND deleted_at IS NOT NULL
    `

	var internalID int64

	err := DB.QueryRow(ctx, query, publicID, userID).Scan(&internalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &internalID, nil
}

func RestoreFile(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		entryID  int64
		parentID sql.NullInt64
		name     string
		fileSize int64
	)

	// Find the trashed file.
	err = tx.QueryRow(ctx, `
		SELECT id, parent_id, name, size
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND type = 'FILE'
		  AND deleted_at IS NOT NULL
	`, publicId, userId).Scan(
		&entryID,
		&parentID,
		&name,
		&fileSize,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find trashed file: %w", err)
	}

	// Lock the trashed file and validate its complete hierarchy.
	// This also verifies that all its parent folders are active.
	err = LockTrashedEntryHierarchy(
		tx,
		ctx,
		userId,
		entryID,
		"FILE",
	)
	if err != nil {
		return fmt.Errorf("failed to lock trashed file hierarchy: %w", err)
	}

	// Restore the file without changing its original parent or quota.
	result, err := tx.Exec(ctx, `
		UPDATE entries
		SET deleted_at = NULL,
		    updated_at = CURRENT_TIMESTAMP,
		    explicitly_deleted = FALSE
		WHERE id = $1
		  AND user_id = $2
		  AND type = 'FILE'
		  AND deleted_at IS NOT NULL
	`, entryID, userId)

	if err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrEntryNotFound
	}

	var parentPtr *int64
	if parentID.Valid {
		parentPtr = &parentID.Int64
	}

	// Insert FILE_RESTORED outbox event.
	eventID := uuid.NewString()

	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFileRestored,
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
	if err != nil {
		return fmt.Errorf("failed to marshal file restored event: %w", err)
	}

	if err := InsertOutboxEvent(
		tx,
		ctx,
		eventID,
		payload,
		"ENTRY",
		publicId,
		events.EventFileRestored,
	); err != nil {
		return fmt.Errorf("failed to insert file restored event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit file restore operation: %w", err)
	}

	return nil
}

func RestoreFolder(ctx context.Context, publicId string, userId int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		folderID int64
		parentID sql.NullInt64
		name     string
	)

	// Find the trashed folder.
	err = tx.QueryRow(ctx, `
		SELECT id, parent_id, name
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND type = 'FOLDER'
		  AND deleted_at IS NOT NULL
	`, publicId, userId).Scan(
		&folderID,
		&parentID,
		&name,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find trashed folder: %w", err)
	}

	// Lock the trashed folder and validate its parent hierarchy.
	// All ancestors must be active before this folder can be restored.
	err = LockTrashedEntryHierarchy(
		tx,
		ctx,
		userId,
		folderID,
		"FOLDER",
	)
	if err != nil {
		return fmt.Errorf("failed to lock trashed folder hierarchy: %w", err)
	}

	// Restore the folder and its implicitly trashed descendants.
	// Independently trashed descendants (explicitly_deleted = TRUE)
	// and their subtrees remain in Trash.
	result, err := tx.Exec(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT
				id,
				explicitly_deleted,
				0 AS depth
			FROM entries
			WHERE id = $1
			  AND user_id = $2
			  AND deleted_at IS NOT NULL

			UNION ALL

			SELECT
				e.id,
				e.explicitly_deleted,
				d.depth + 1
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
			  AND e.deleted_at IS NOT NULL
			  AND (d.depth = 0 OR d.explicitly_deleted = FALSE)
		)
		UPDATE entries
		SET deleted_at = NULL,
		    updated_at = CURRENT_TIMESTAMP,
		    explicitly_deleted = FALSE
		WHERE id IN (
			SELECT id
			FROM descendants
			WHERE depth = 0 OR explicitly_deleted = FALSE
		)
		  AND user_id = $2
		  AND deleted_at IS NOT NULL
	`, folderID, userId)

	if err != nil {
		return fmt.Errorf("failed to restore folder and descendants: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrEntryNotFound
	}

	var parentPtr *int64
	if parentID.Valid {
		parentPtr = &parentID.Int64
	}

	// Insert FOLDER_RESTORED outbox event.
	eventID := uuid.NewString()

	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFolderRestored,
		AggregateID: publicId,
		UserID:      userId,
		EntryID:     folderID,
		PublicID:    publicId,
		ParentID:    parentPtr,
		Name:        name,
		EntryType:   "FOLDER",
		Size:        0,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal folder restored event: %w", err)
	}

	if err := InsertOutboxEvent(
		tx,
		ctx,
		eventID,
		payload,
		"ENTRY",
		publicId,
		events.EventFolderRestored,
	); err != nil {
		return fmt.Errorf("failed to insert folder restored event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit folder restore operation: %w", err)
	}

	return nil
}

// LockPermanentDeleteHierarchy locks the target's complete ancestor hierarchy.
//
// Unlike LockTrashedEntryHierarchy, this allows trashed ancestors because
// an item can be permanently deleted from inside a trashed parent folder.
//
// Ancestors are locked FOR SHARE and the target FOR UPDATE, always root -> target.
func LockPermanentDeleteHierarchy(
	tx pgx.Tx,
	ctx context.Context,
	userID int64,
	entryID int64,
	expectedTargetType string,
) error {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT
				id,
				parent_id,
				type,
				ARRAY[id]::BIGINT[] AS path
			FROM entries
			WHERE id = $1
			  AND user_id = $2

			UNION ALL

			SELECT
				p.id,
				p.parent_id,
				p.type,
				a.path || p.id
			FROM entries p
			JOIN ancestors a ON a.parent_id = p.id
			WHERE p.user_id = $2
			  AND NOT p.id = ANY(a.path)
		)
		SELECT id
		FROM ancestors
		ORDER BY cardinality(path) DESC
	`, entryID, userID)
	if err != nil {
		return fmt.Errorf("failed to read entry hierarchy: %w", err)
	}

	var hierarchyIDs []int64

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		hierarchyIDs = append(hierarchyIDs, id)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(hierarchyIDs) == 0 || hierarchyIDs[len(hierarchyIDs)-1] != entryID {
		return ErrEntryNotFound
	}

	// Acquire locks in root -> target order and validate the current rows
	// after obtaining each lock.
	var previousID int64
	var previousType string

	for i, id := range hierarchyIDs {
		var (
			lockedID  int64
			parentID  *int64
			entryType string
			deletedAt sql.NullTime
		)

		if i == len(hierarchyIDs)-1 {
			err = tx.QueryRow(ctx, `
				SELECT id, parent_id, type, deleted_at
				FROM entries
				WHERE id = $1 AND user_id = $2
				FOR UPDATE
			`, id, userID).Scan(
				&lockedID, &parentID, &entryType, &deletedAt,
			)
		} else {
			err = tx.QueryRow(ctx, `
				SELECT id, parent_id, type, deleted_at
				FROM entries
				WHERE id = $1 AND user_id = $2
				FOR SHARE
			`, id, userID).Scan(
				&lockedID, &parentID, &entryType, &deletedAt,
			)
		}

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEntryNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock hierarchy entry: %w", err)
		}

		if i == 0 {
			if parentID != nil {
				return ErrInvalidEntryHierarchy
			}
		} else {
			if parentID == nil || *parentID != previousID || previousType != "FOLDER" {
				return ErrInvalidEntryHierarchy
			}
		}

		if i == len(hierarchyIDs)-1 {
			if entryType != expectedTargetType {
				return ErrInvalidEntryHierarchy
			}
			if !deletedAt.Valid {
				return ErrEntryNotFound
			}
		}

		previousID = lockedID
		previousType = entryType
	}

	return nil
}

// PermanentlyDeleteEntry permanently deletes an item that is already in Trash.
//
// For folders, the complete subtree is deleted, including independently
// trashed descendants. Quota release and the S3 cleanup outbox event are
// committed atomically with the database deletion.
func PermanentlyDeleteEntry(ctx context.Context, publicID string, userID int64) error {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin permanent delete transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		entryID   int64
		entryType string
	)

	err = tx.QueryRow(ctx, `
		SELECT id, type
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND deleted_at IS NOT NULL
	`, publicID, userID).Scan(&entryID, &entryType)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to find trashed entry: %w", err)
	}

	// Lock and validate the target hierarchy. Trashed ancestors are allowed.
	if err := LockPermanentDeleteHierarchy(tx, ctx, userID, entryID, entryType); err != nil {
		return fmt.Errorf("failed to lock permanent delete hierarchy: %w", err)
	}

	// Collect every descendant, including already-trashed descendants.
	// Do not filter by deleted_at here.
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT
				id,
				type,
				size,
				s3_key,
				ARRAY[id]::BIGINT[] AS path
			FROM entries
			WHERE id = $1
			  AND user_id = $2

			UNION ALL

			SELECT
				e.id,
				e.type,
				e.size,
				e.s3_key,
				s.path || e.id
			FROM entries e
			JOIN subtree s ON e.parent_id = s.id
			WHERE e.user_id = $2
			  AND NOT e.id = ANY(s.path)
		)
		SELECT type, size, s3_key
		FROM subtree
	`, entryID, userID)
	if err != nil {
		return fmt.Errorf("failed to collect subtree: %w", err)
	}

	var (
		totalSize int64
		objects   []events.S3CleanupObject
	)

	bucketName := config.AppConfig.BucketName

	for rows.Next() {
		var (
			itemType string
			size     int64
			s3Key    sql.NullString
		)

		if err := rows.Scan(&itemType, &size, &s3Key); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan subtree entry: %w", err)
		}

		if itemType == "FILE" {
			totalSize += size

			if s3Key.Valid && s3Key.String != "" {
				objects = append(objects, events.S3CleanupObject{
					BucketName: bucketName,
					S3Key:      s3Key.String,
				})
			}
		}
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("failed to read subtree: %w", err)
	}
	rows.Close()

	if len(objects) > 0 && bucketName == "" {
		return fmt.Errorf("S3 bucket name cannot be empty")
	}

	// Release quota in the same transaction as the permanent deletion.
	if totalSize > 0 {
		if _, err := ReleaseQuota(tx, ctx, userID, totalSize); err != nil {
			return fmt.Errorf("failed to release storage quota: %w", err)
		}
	}

	// ON DELETE CASCADE permanently removes all descendants.
	result, err := tx.Exec(ctx, `
		DELETE FROM entries
		WHERE id = $1
		  AND user_id = $2
		  AND deleted_at IS NOT NULL
	`, entryID, userID)
	if err != nil {
		return fmt.Errorf("failed to permanently delete entry: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrEntryNotFound
	}

	// Only create an S3 cleanup event when there are actual objects to delete.
	if len(objects) > 0 {
		eventID := uuid.NewString()

		payload, err := json.Marshal(events.S3CleanupEvent{
			EventID:   eventID,
			EventType: events.EventS3Cleanup,
			UserID:    userID,
			Objects:   objects,
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			return fmt.Errorf("failed to marshal S3 cleanup event: %w", err)
		}

		if err := InsertOutboxEvent(
			tx,
			ctx,
			eventID,
			payload,
			"S3_CLEANUP",
			publicID,
			events.EventS3Cleanup,
		); err != nil {
			return fmt.Errorf("failed to insert S3 cleanup outbox event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit permanent delete transaction: %w", err)
	}

	return nil
}
