package models

import (
	"database/sql"
	"time"
)

type ListFileResponse struct {
	PublicId    string    `json:"public_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	ContentType string    `json:"content_type"`
	Extension   string    `json:"extension"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type EntryData struct {
	Id          int64          `db:"id"`
	PublicId    string         `db:"public_id"`
	UserId      int64          `db:"user_id"`
	ParentId    *int64         `db:"parent_id"`
	Name        string         `db:"name"`
	Type        string         `db:"type"`
	ContentType string         `db:"content_type"`
	Extension   string         `db:"extension"`
	Size        int64          `db:"size"`
	S3Key       sql.NullString `db:"s3_key"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
	DeletedAt   time.Time      `db:"deleted_at"`
}

type DeleteFile struct {
	Name  string `db:"name"`
	Type  string `db:"type"`
	S3Key string `db:"s3_key"`
}

type SearchResponse struct {
	ID       int64   `json:"id"`
	PublicID string  `json:"public_id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Score    float64 `json:"score"` // Similarity percentage
}
