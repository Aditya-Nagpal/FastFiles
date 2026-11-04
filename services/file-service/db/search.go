package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/pgvector/pgvector-go"
)

func SearchByVector(ctx context.Context, userId int64, vector pgvector.Vector, limit int) ([]models.ListFileResponse, error) {
	query := `
		SELECT * FROM (
			SELECT DISTINCT ON (e.id)
				e.public_id, e.name, e.type, e.content_type, e.extension, e.size, e.created_at, e.updated_at,
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

	files := make([]models.ListFileResponse, 0)
	for rows.Next() {
		var file models.ListFileResponse
		var score float64
		if err := rows.Scan(&file.PublicId, &file.Name, &file.Type, &file.ContentType, &file.Extension, &file.Size, &file.CreatedAt, &file.UpdatedAt, &score); err != nil {
			return nil, err
		}

		if file.Extension != "" {
			file.Name = file.Name + "." + file.Extension
		}

		files = append(files, file)
	}
	return files, nil
}

func SearchByName(ctx context.Context, userID int64, query string, limit int) ([]models.ListFileResponse, error) {

	// Escape LIKE wildcard characters so user input is treated literally.
	escapedQuery := strings.NewReplacer(
		"!", "!!",
		"%", "!%",
		"_", "!_",
	).Replace(query)

	prefixPattern := escapedQuery + "%"
	substringPattern := "%" + escapedQuery + "%"

	searchPattern := substringPattern

	// Short queries use prefix matching.
	if len([]rune(query)) < 3 {
		searchPattern = prefixPattern
	}

	sqlQuery := `
		SELECT
			e.public_id, e.name, e.type, e.content_type, e.extension, e.size, e.created_at, e.updated_at
		FROM entries e
		WHERE e.user_id = $1
			AND e.deleted_at IS NULL
			AND e.name ILIKE $2 ESCAPE '!'
		ORDER BY
			CASE
				WHEN LOWER(e.name) = LOWER($3) THEN 1
				WHEN e.name ILIKE $4 ESCAPE '!' THEN 2
				ELSE 3
			END,
			e.name ASC
		LIMIT $5
	`

	rows, err := DB.Query(ctx, sqlQuery, userID, searchPattern, query, prefixPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search entries by name: %w", err)
	}
	defer rows.Close()

	results := make([]models.ListFileResponse, 0)

	for rows.Next() {
		var result models.ListFileResponse

		err := rows.Scan(
			&result.PublicId,
			&result.Name,
			&result.Type,
			&result.ContentType,
			&result.Extension,
			&result.Size,
			&result.CreatedAt,
			&result.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan named search result: %w", err)
		}

		if result.Extension != "" {
			result.Name = result.Name + "." + result.Extension
		}

		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read named search results: %w", err)
	}

	return results, nil
}
