package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// fakeS3PutObjectClient simulates an S3 upload failure without AWS.
type fakeS3PutObjectClient struct {
	err error
}

func (f *fakeS3PutObjectClient) PutObject(
	ctx context.Context,
	params *s3.PutObjectInput,
	optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	return nil, f.err
}

func TestUploadFile_S3FailureReleasesReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set; configure it or services/file-service/.env")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	// Use an isolated test user and storage record.
	userID := time.Now().UnixNano()
	publicID := fmt.Sprintf("f%019d", userID%1_000_000_000_000_000_000)

	previousDB := db.DB
	db.DB = pool
	defer func() {
		db.DB = previousDB
	}()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = $1`,
			publicID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create test storage record: %v", err)
	}

	// Prepare a multipart file upload request.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "failure-test.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	const fileContents = "simulate an S3 upload failure"
	if _, err := part.Write([]byte(fileContents)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	// Inject a fake S3 client that always fails.
	expectedS3Error := errors.New("simulated S3 PutObject failure")

	uploader := &utils.S3Uploader{
		PutObjectClient: &fakeS3PutObjectClient{
			err: expectedS3Error,
		},
		BucketName: "test-bucket",
	}

	// Call the handler's core upload function directly.
	err = UploadFile(c, uploader, nil, userID, publicID, nil)

	if !errors.Is(err, expectedS3Error) {
		t.Fatalf("expected simulated S3 error, got: %v", err)
	}

	// Verify that reservation cleanup happened.
	var usedBytes, reservedBytes int64
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read storage usage: %v", err)
	}

	if usedBytes != 0 {
		t.Errorf("used_bytes = %d; want 0", usedBytes)
	}
	if reservedBytes != 0 {
		t.Errorf("reserved_bytes = %d; want 0", reservedBytes)
	}

	// Verify that no file metadata was inserted.
	var entryCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1 AND public_id = $2 AND type = 'FILE'
	`, userID, publicID).Scan(&entryCount)
	if err != nil {
		t.Fatalf("check file metadata: %v", err)
	}

	if entryCount != 0 {
		t.Errorf("found %d file entries after S3 upload failure; want 0", entryCount)
	}

	// The fake should be the only S3 dependency needed for this test.
	_ = types.ObjectCannedACLPublicRead

	t.Logf(
		"PASS: S3 upload failed; used_bytes=%d, reserved_bytes=%d, file entries=%d",
		usedBytes,
		reservedBytes,
		entryCount,
	)
}

type fakeS3DeleteObjectClient struct {
	calls int
	err   error
}

func (f *fakeS3DeleteObjectClient) DeleteObject(
	ctx context.Context,
	params *s3.DeleteObjectInput,
	optFns ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	f.calls++
	return &s3.DeleteObjectOutput{}, f.err
}

func TestUploadFile_DBFailureCleansUpS3AndReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%019d", userID%1_000_000_000_000_000_000)
	folderPublicID := "r" + suffix
	filePublicID := "f" + suffix

	previousDB := db.DB
	db.DB = pool
	defer func() {
		db.DB = previousDB
	}()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{folderPublicID, filePublicID},
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`, userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`, userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create storage record: %v", err)
	}

	// Create a real destination folder.
	var folderID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'test-folder', 'FOLDER', '')
		RETURNING id
	`, folderPublicID, userID).Scan(&folderID)
	if err != nil {
		t.Fatalf("create test folder: %v", err)
	}

	// Make the destination inactive so DB finalization must fail.
	_, err = pool.Exec(ctx, `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2
	`, folderID, userID)
	if err != nil {
		t.Fatalf("soft-delete test folder: %v", err)
	}

	// Build a multipart upload request.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "db-failure-test.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte("test content")); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	// Upload succeeds, but database finalization will fail.
	fakeDelete := &fakeS3DeleteObjectClient{}
	uploader := &utils.S3Uploader{
		PutObjectClient:    &fakeS3PutObjectClient{},
		DeleteObjectClient: fakeDelete,
		BucketName:         "test-bucket",
	}

	err = UploadFile(c, uploader, nil, userID, filePublicID, &folderID)
	if err == nil {
		t.Fatal("expected DB finalization to fail for a deleted destination folder")
	}

	if fakeDelete.calls != 1 {
		t.Errorf("expected one S3 cleanup attempt, got %d", fakeDelete.calls)
	}

	// Reservation should have been released by UploadFile's deferred cleanup.
	var usedBytes, reservedBytes int64
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read storage usage: %v", err)
	}

	if usedBytes != 0 {
		t.Errorf("used_bytes = %d; want 0", usedBytes)
	}
	if reservedBytes != 0 {
		t.Errorf("reserved_bytes = %d; want 0", reservedBytes)
	}

	var fileCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1
		  AND public_id = $2
		  AND type = 'FILE'
	`, userID, filePublicID).Scan(&fileCount)
	if err != nil {
		t.Fatalf("check file metadata: %v", err)
	}
	if fileCount != 0 {
		t.Errorf("expected no file metadata, found %d entries", fileCount)
	}

	t.Logf(
		"PASS: DB failure=%v, cleanup calls=%d, used=%d, reserved=%d, file entries=%d",
		err, fakeDelete.calls, usedBytes, reservedBytes, fileCount,
	)
}

func TestUploadFile_ReservationReleaseFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%d", userID)
	publicID := "f" + suffix

	triggerName := "test_fail_release_" + suffix
	functionName := "test_fail_release_fn_" + suffix

	previousDB := db.DB
	db.DB = pool
	defer func() {
		db.DB = previousDB
	}()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		// Remove the test trigger before attempting reservation cleanup.
		_, _ = pool.Exec(cleanupCtx,
			fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON user_storage`, triggerName))
		_, _ = pool.Exec(cleanupCtx,
			fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))

		// Best-effort cleanup in case the test exits early.
		var reserved int64
		err := pool.QueryRow(cleanupCtx,
			`SELECT reserved_bytes FROM user_storage WHERE user_id = $1`,
			userID,
		).Scan(&reserved)
		if err == nil && reserved > 0 {
			_ = db.ReleaseStorageReservation(cleanupCtx, userID, reserved)
		}

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = $1`, publicID)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`, userID)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`, userID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create storage record: %v", err)
	}

	// Allow reservation increases, but reject reservation releases
	// for this specific test user.
	createFunctionSQL := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.user_id = TG_ARGV[0]::BIGINT
			   AND NEW.reserved_bytes < OLD.reserved_bytes
			   AND NEW.used_bytes = OLD.used_bytes THEN
				RAISE EXCEPTION 'simulated reservation release failure';
			END IF;
			RETURN NEW;
		END;
		$$
	`, functionName)

	if _, err := pool.Exec(ctx, createFunctionSQL); err != nil {
		t.Fatalf("create test trigger function: %v", err)
	}

	createTriggerSQL := fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE UPDATE ON user_storage
		FOR EACH ROW
		EXECUTE FUNCTION %s('%d')
	`, triggerName, functionName, userID)

	if _, err := pool.Exec(ctx, createTriggerSQL); err != nil {
		t.Fatalf("create test trigger: %v", err)
	}

	// Prepare a multipart upload.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "release-failure.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	const contents = "test reservation release failure"
	if _, err := part.Write([]byte(contents)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request

	// Force S3 upload failure. The deferred reservation release will
	// encounter the test trigger and fail too.
	s3Failure := errors.New("simulated S3 upload failure")
	uploader := &utils.S3Uploader{
		PutObjectClient: &fakeS3PutObjectClient{err: s3Failure},
		BucketName:      "test-bucket",
	}

	uploadErr := UploadFile(c, uploader, nil, userID, publicID, nil)
	if !errors.Is(uploadErr, s3Failure) {
		t.Fatalf("expected S3 failure, got: %v", uploadErr)
	}

	var usedBytes, reservedBytes int64
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read storage usage: %v", err)
	}

	fileSize := int64(len(contents))

	if usedBytes != 0 {
		t.Errorf("used_bytes = %d; want 0", usedBytes)
	}
	if reservedBytes != fileSize {
		t.Errorf(
			"reserved_bytes = %d; want %d because release was deliberately blocked",
			reservedBytes, fileSize,
		)
	}

	// Remove the trigger and recover the reservation so the test
	// leaves no reserved quota behind.
	if _, err := pool.Exec(ctx,
		fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON user_storage`, triggerName)); err != nil {
		t.Fatalf("drop test trigger: %v", err)
	}

	if _, err := pool.Exec(ctx,
		fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName)); err != nil {
		t.Fatalf("drop test function: %v", err)
	}

	if err := db.ReleaseStorageReservation(ctx, userID, fileSize); err != nil {
		t.Fatalf("release reservation after removing test trigger: %v", err)
	}

	t.Logf(
		"PASS: S3 failed; reservation release was blocked; used=%d, reserved before recovery=%d; recovery succeeded",
		usedBytes, reservedBytes,
	)
}
