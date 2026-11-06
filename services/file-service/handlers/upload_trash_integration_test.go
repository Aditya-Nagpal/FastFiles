package handlers

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type blockingS3Client struct {
	putStarted   chan struct{}
	allowPut     chan struct{}
	deleteCalled chan string
}

func (f *blockingS3Client) PutObject(
	ctx context.Context,
	input *s3.PutObjectInput,
	optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	close(f.putStarted)

	select {
	case <-f.allowPut:
		return &s3.PutObjectOutput{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *blockingS3Client) DeleteObject(
	ctx context.Context,
	input *s3.DeleteObjectInput,
	optFns ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	f.deleteCalled <- *input.Key
	return &s3.DeleteObjectOutput{}, nil
}

func findHandlerTestEnvFile() string {
	candidates := []string{
		".env",
		"../.env",
		"../../.env",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func TestUploadFile_ParentTrashedDuringS3Upload(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	envPath := findHandlerTestEnvFile()
	if envPath == "" {
		t.Fatal(".env file not found")
	}

	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not configured")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	previousDB := db.DB
	db.DB = pool

	t.Cleanup(func() {
		db.DB = previousDB
		pool.Close()
	})

	// testID := uuid.NewString()
	userID := time.Now().UnixNano()

	rootPublicID := uuid.NewString()[:21]
	filePublicID := uuid.NewString()[:21]

	const initialUsedBytes int64 = 512
	const uploadSize int64 = 128

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id, quota_bytes, used_bytes, reserved_bytes
		)
		VALUES ($1, 10485760, $2, 0)
	`, userID, initialUsedBytes)
	if err != nil {
		t.Fatalf("failed to create storage fixture: %v", err)
	}

	var rootID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id, user_id, parent_id, name,
			type, content_type, extension, size, s3_key
		)
		VALUES ($1, $2, NULL, 'root', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatalf("failed to create folder fixture: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_id = $1
		`, rootPublicID)
		if err != nil {
			t.Errorf("failed to clean outbox: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM entries WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean entries: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM storage_reservations WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean reservations: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean storage: %v", err)
		}
	})

	// Fake S3 client: upload pauses until the test trashes the parent.
	fakeS3 := &blockingS3Client{
		putStarted:   make(chan struct{}),
		allowPut:     make(chan struct{}),
		deleteCalled: make(chan string, 1),
	}

	uploader := &utils.S3Uploader{
		PutObjectClient:    fakeS3,
		DeleteObjectClient: fakeS3,
		BucketName:         "integration-test-bucket",
	}

	// Build a real multipart request.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "race.txt")
	if err != nil {
		t.Fatalf("failed to create multipart file: %v", err)
	}

	if _, err := part.Write(bytes.Repeat([]byte("x"), int(uploadSize))); err != nil {
		t.Fatalf("failed to write multipart file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/upload",
		body,
	)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = request

	uploadResult := make(chan error, 1)

	// Start the actual production upload handler flow.
	go func() {
		uploadResult <- uploadFileWithHeartbeat(
			ginContext,
			uploader,
			nil, // Not a PDF, so TaskService is not needed.
			userID,
			filePublicID,
			&rootID,
			time.Hour,
			2*time.Hour,
			nil,
		)
	}()

	// Wait until the fake S3 client receives PutObject.
	select {
	case <-fakeS3.putStarted:
	case <-ctx.Done():
		t.Fatal("upload did not reach S3 PutObject")
	}

	// Trash the parent while the S3 upload is still paused.
	if err := db.SoftDeleteFolder(ctx, rootPublicID, userID); err != nil {
		t.Fatalf("SoftDeleteFolder failed: %v", err)
	}

	// Allow S3 upload to finish.
	close(fakeS3.allowPut)

	// DB finalization must fail because the parent is now trashed.
	uploadErr := <-uploadResult

	if !errors.Is(uploadErr, db.ErrEntryNotFound) {
		t.Fatalf(
			"expected ErrEntryNotFound from upload, got: %v",
			uploadErr,
		)
	}

	// The cleanup helper must delete the uploaded S3 object.
	expectedS3Key := utils.CreateS3Key(
		userID,
		filePublicID,
		"txt",
	)

	select {
	case deletedKey := <-fakeS3.deleteCalled:
		if deletedKey != expectedS3Key {
			t.Fatalf(
				"expected S3 cleanup key %q, got %q",
				expectedS3Key,
				deletedKey,
			)
		}
	case <-ctx.Done():
		t.Fatal("S3 cleanup was not called")
	}

	// Verify no file entry was committed.
	var fileCount int

	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
	`, filePublicID, userID).Scan(&fileCount)
	if err != nil {
		t.Fatalf("failed to verify file entry: %v", err)
	}

	if fileCount != 0 {
		t.Fatalf("expected no file entry, found %d", fileCount)
	}

	// Verify quota and reservation cleanup.
	var usedBytes, reservedBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("failed to read storage quota: %v", err)
	}

	if usedBytes != initialUsedBytes {
		t.Fatalf(
			"expected used_bytes=%d, got %d",
			initialUsedBytes,
			usedBytes,
		)
	}

	if reservedBytes != 0 {
		t.Fatalf("expected reserved_bytes=0, got %d", reservedBytes)
	}

	var reservationStatus string

	err = pool.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE user_id = $1
		ORDER BY id DESC
		LIMIT 1
	`, userID).Scan(&reservationStatus)
	if err != nil {
		t.Fatalf("failed to read reservation status: %v", err)
	}

	if reservationStatus != "RELEASED" {
		t.Fatalf(
			"expected reservation RELEASED, got %s",
			reservationStatus,
		)
	}

	t.Log(
		"SUCCESS: parent was trashed during S3 upload; " +
			"DB finalization rejected the file; S3 cleanup was called; " +
			"reservation was released",
	)
}
