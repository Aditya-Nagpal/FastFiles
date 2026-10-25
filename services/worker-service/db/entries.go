package db

import (
	"context"
	"fmt"
)

func RecalculateAncestorFolderSizes(ctx context.Context, userID int64, entryID int64) error {
	query := `
		WITH RECURSIVE changed_entry AS (
			SELECT parent_id
			FROM entries
			WHERE id = $1
			  AND user_id = $2
		),
		ancestors AS (
			SELECT p.id, p.parent_id
			FROM entries p
			JOIN changed_entry c ON c.parent_id = p.id
			WHERE p.user_id = $2
			  AND p.type = 'FOLDER'
			  AND p.deleted_at IS NULL

			UNION ALL

			SELECT p.id, p.parent_id
			FROM entries p
			JOIN ancestors a ON p.id = a.parent_id
			WHERE p.user_id = $2
			  AND p.type = 'FOLDER'
			  AND p.deleted_at IS NULL
		),
		subtree AS (
			SELECT
				a.id AS ancestor_id,
				e.id AS entry_id,
				e.type,
				e.size
			FROM ancestors a
			JOIN entries e ON e.parent_id = a.id
			WHERE e.user_id = $2
			  AND e.deleted_at IS NULL

			UNION ALL

			SELECT
				s.ancestor_id,
				e.id AS entry_id,
				e.type,
				e.size
			FROM subtree s
			JOIN entries e ON e.parent_id = s.entry_id
			WHERE e.user_id = $2
			  AND e.deleted_at IS NULL
		),
		folder_sizes AS (
			SELECT
				a.id AS folder_id,
				COALESCE(
					SUM(s.size) FILTER (WHERE s.type = 'FILE'),
					0
				)::BIGINT AS total_size
			FROM ancestors a
			LEFT JOIN subtree s ON s.ancestor_id = a.id
			GROUP BY a.id
		)
		UPDATE entries f
		SET size = fs.total_size,
		    updated_at = CURRENT_TIMESTAMP
		FROM folder_sizes fs
		WHERE f.id = fs.folder_id
		  AND f.user_id = $2
		  AND f.type = 'FOLDER'
		  AND f.deleted_at IS NULL
	`
	_, err := DB.Exec(ctx, query, entryID, userID)

	if err != nil {
		return fmt.Errorf("failed to recalculate ancestor folder sizes: %w", err)
	}

	return nil
}

func GetSingleFileKey(ctx context.Context, userID int64, entryID int64) (string, error) {
	query := `
		SELECT s3_key
		FROM entries
		WHERE id = $1
			AND user_id = $2
			AND type = 'FILE'
	`

	var key string
	err := DB.QueryRow(ctx, query, entryID, userID).Scan(&key)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file S3 key: %w", err)
	}

	return key, nil
}

func GetDescendantFileKeys(ctx context.Context, userID int64, entryID int64) ([]string, error) {
	query := `
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, type, s3_key
			FROM entries
			WHERE id = $1
				AND user_id = $2
				AND type = 'FOLDER'

			UNION ALL

			SELECT e.id, e.parent_id, e.type, e.s3_key
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
		)
		SELECT s3_key
		FROM descendants
		WHERE type = 'FILE'
			AND s3_key IS NOT NULL
			AND s3_key <> ''
	`

	rows, err := DB.Query(ctx, query, entryID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch descendant file S3 keys: %w", err)
	}

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan S3 key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("failed reading descendant S3 keys: %w", err)
	}
	rows.Close()

	return keys, nil
}
