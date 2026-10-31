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
	"sync"
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
			userID,
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
			`DELETE FROM storage_reservations WHERE user_id = $1`, userID)

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

type blockingFakeS3PutObjectClient struct {
	started chan struct{}
	finish  chan struct{}
}

func (f *blockingFakeS3PutObjectClient) PutObject(
	ctx context.Context,
	params *s3.PutObjectInput,
	optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	close(f.started)

	select {
	case <-f.finish:
		return &s3.PutObjectOutput{}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestUploadFile_LongRunningUploadKeepsReservationAlive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
			userID,
		)

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM storage_reservations WHERE user_id = $1`,
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
		t.Fatalf("create storage record: %v", err)
	}

	// Prepare multipart upload.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "long-running-test.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	const fileContents = "simulated long-running upload"
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

	// Fake S3 upload blocks until the test allows it to finish.
	fakeS3 := &blockingFakeS3PutObjectClient{
		started: make(chan struct{}),
		finish:  make(chan struct{}),
	}

	uploader := &utils.S3Uploader{
		PutObjectClient: fakeS3,
		BucketName:      "test-bucket",
	}

	uploadDone := make(chan struct{})

	var (
		uploadErr  error
		finishOnce sync.Once
	)

	finishUpload := func() {
		finishOnce.Do(func() {
			close(fakeS3.finish)
		})
	}

	// Use short test-only lease and heartbeat intervals.
	go func() {
		defer close(uploadDone)

		uploadErr = uploadFileWithHeartbeat(
			c,
			uploader,
			nil,
			userID,
			publicID,
			nil,
			50*time.Millisecond,
			300*time.Millisecond,
			db.RenewStorageReservation,
		)
	}()

	// Always stop and wait for the upload goroutine before
	// restoring DB or closing the PostgreSQL pool.
	defer func() {
		finishUpload()

		select {
		case <-uploadDone:
		case <-time.After(3 * time.Second):
			t.Error("upload goroutine did not terminate")
		}
	}()

	// Wait until S3 upload starts.
	// Do NOT finish the upload yet.
	select {
	case <-fakeS3.started:
		// Upload is blocked inside fake S3.

	case <-ctx.Done():
		t.Fatal("S3 upload did not start")
	}

	// Wait longer than the original 300ms lease.
	// Several heartbeat renewals should happen during this time.
	time.Sleep(450 * time.Millisecond)

	var reservationID int64
	var leaseExpiresAt time.Time

	err = pool.QueryRow(ctx, `
		SELECT id, lease_expires_at
		FROM storage_reservations
		WHERE user_id = $1
		  AND status = 'PENDING'
	`, userID).Scan(&reservationID, &leaseExpiresAt)
	if err != nil {
		t.Fatalf("read pending reservation: %v", err)
	}

	// The lease must have been extended beyond its original expiry.
	if !leaseExpiresAt.After(time.Now().Add(100 * time.Millisecond)) {
		t.Fatalf(
			"reservation lease was not renewed sufficiently; expires at %v",
			leaseExpiresAt,
		)
	}

	// Run the real recovery function while the upload is still active.
	recovered, err := db.RecoverExpiredStorageReservations(ctx, 100)
	if err != nil {
		t.Fatalf("run reservation recovery: %v", err)
	}

	if recovered != 0 {
		t.Fatalf(
			"recovery released %d reservations during an active upload; want 0",
			recovered,
		)
	}

	// Confirm reservation is still pending.
	var status string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE id = $1
	`, reservationID).Scan(&status)
	if err != nil {
		t.Fatalf("read reservation status: %v", err)
	}

	if status != "PENDING" {
		t.Fatalf("reservation status = %s; want PENDING", status)
	}

	finishUpload()

	select {
	case <-uploadDone:
		if uploadErr != nil {
			t.Fatalf("upload failed: %v", uploadErr)
		}

	case <-ctx.Done():
		t.Fatal("upload did not finish before timeout")
	}

	// Verify final quota accounting.
	var usedBytes, reservedBytes int64
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read storage usage: %v", err)
	}

	expectedSize := int64(len(fileContents))

	if usedBytes != expectedSize {
		t.Errorf("used_bytes = %d; want %d", usedBytes, expectedSize)
	}

	if reservedBytes != 0 {
		t.Errorf("reserved_bytes = %d; want 0", reservedBytes)
	}

	err = pool.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE id = $1
	`, reservationID).Scan(&status)
	if err != nil {
		t.Fatalf("read finalized reservation: %v", err)
	}

	if status != "FINALIZED" {
		t.Errorf("reservation status = %s; want FINALIZED", status)
	}

	t.Logf(
		"PASS: long upload survived original lease expiry; recovery=%d, used=%d, reserved=%d, status=%s",
		recovered,
		usedBytes,
		reservedBytes,
		status,
	)
}

func TestUploadFile_HeartbeatFailureCancelsUploadAndReleasesReservation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
			userID,
		)

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	// Create storage record.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create storage record: %v", err)
	}

	// Prepare multipart upload.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "heartbeat-failure.txt")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}

	const fileContents = "simulate heartbeat renewal failure"

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

	// Fake S3 upload waits until its context is cancelled.
	fakeS3 := &blockingFakeS3PutObjectClient{
		started: make(chan struct{}),
		finish:  make(chan struct{}),
	}

	uploader := &utils.S3Uploader{
		PutObjectClient: fakeS3,
		BucketName:      "test-bucket",
	}

	// Simulate a database failure during heartbeat renewal.
	expectedRenewalError := errors.New("simulated database renewal failure")

	renewalCalled := make(chan struct{}, 1)

	renew := func(
		renewCtx context.Context,
		reservationID int64,
		leaseDuration time.Duration,
	) error {
		renewalCalled <- struct{}{}
		return expectedRenewalError
	}

	uploadDone := make(chan error, 1)

	go func() {
		uploadDone <- uploadFileWithHeartbeat(
			c,
			uploader,
			nil,
			userID,
			publicID,
			nil,
			20*time.Millisecond,
			300*time.Millisecond,
			renew,
		)
	}()

	// Wait until S3 upload starts.
	select {
	case <-fakeS3.started:
	case <-ctx.Done():
		t.Fatal("S3 upload did not start")
	}

	// Wait until heartbeat attempts renewal.
	select {
	case <-renewalCalled:
	case <-ctx.Done():
		t.Fatal("heartbeat did not attempt renewal")
	}

	// Upload should be cancelled after renewal failure.
	select {
	case err := <-uploadDone:
		if err == nil {
			t.Fatal("expected upload to fail after heartbeat renewal failure")
		}

		if !errors.Is(err, expectedRenewalError) {
			t.Fatalf(
				"expected heartbeat error %v, got %v",
				expectedRenewalError,
				err,
			)
		}

	case <-ctx.Done():
		t.Fatal("upload did not stop after heartbeat renewal failure")
	}

	// Verify reservation cleanup.
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

	// Verify reservation status.
	var status string

	err = pool.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE user_id = $1
	`, userID).Scan(&status)
	if err != nil {
		t.Fatalf("read reservation status: %v", err)
	}

	if status != "RELEASED" {
		t.Errorf("reservation status = %s; want RELEASED", status)
	}

	// No file metadata should have been inserted.
	var entryCount int

	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1
		  AND public_id = $2
		  AND type = 'FILE'
	`, userID, publicID).Scan(&entryCount)
	if err != nil {
		t.Fatalf("check file metadata: %v", err)
	}

	if entryCount != 0 {
		t.Errorf("found %d file entries; want 0", entryCount)
	}

	t.Logf(
		"PASS: renewal failure cancelled upload; status=%s, used=%d, reserved=%d, entries=%d",
		status,
		usedBytes,
		reservedBytes,
		entryCount,
	)
}

func TestAbandonedUploadReservationRecoveredAfterCrash(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
			userID,
		)

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	const (
		fileSize = int64(1024)
		quota    = int64(1 << 30)
	)

	// 1. Create user storage record.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, quota)
	if err != nil {
		t.Fatalf("create storage record: %v", err)
	}

	// 2. Create reservation, just like an upload would.
	reservationID, err := db.CreateStorageReservation(
		ctx,
		userID,
		fileSize,
	)
	if err != nil {
		t.Fatalf("create storage reservation: %v", err)
	}

	// Verify quota was reserved.
	var usedBytes, reservedBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read initial storage usage: %v", err)
	}

	if usedBytes != 0 || reservedBytes != fileSize {
		t.Fatalf(
			"unexpected initial quota: used=%d reserved=%d",
			usedBytes,
			reservedBytes,
		)
	}

	// 3. Simulate application crash:
	// No heartbeat is running, and the reservation is abandoned.
	// Force its lease to expire.
	_, err = pool.Exec(ctx, `
		UPDATE storage_reservations
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second',
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PENDING'
	`, reservationID)
	if err != nil {
		t.Fatalf("expire abandoned reservation: %v", err)
	}

	// 4. Run the real recovery function.
	recovered, err := db.RecoverExpiredStorageReservations(ctx, 100)
	if err != nil {
		t.Fatalf("recover abandoned reservation: %v", err)
	}

	if recovered != 1 {
		t.Fatalf("recovered %d reservations; want 1", recovered)
	}

	// 5. Verify reservation status.
	var status string

	err = pool.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE id = $1
	`, reservationID).Scan(&status)
	if err != nil {
		t.Fatalf("read reservation status: %v", err)
	}

	if status != "RELEASED" {
		t.Fatalf("reservation status = %s; want RELEASED", status)
	}

	// 6. Verify quota was released.
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read final storage usage: %v", err)
	}

	if usedBytes != 0 {
		t.Errorf("used_bytes = %d; want 0", usedBytes)
	}

	if reservedBytes != 0 {
		t.Errorf("reserved_bytes = %d; want 0", reservedBytes)
	}

	t.Logf(
		"PASS: abandoned upload recovered; reservation=%d, status=%s, used=%d, reserved=%d",
		reservationID,
		status,
		usedBytes,
		reservedBytes,
	)
}

func TestConcurrentRecoveryWorkers(t *testing.T) {
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
			`DELETE FROM storage_reservations WHERE user_id = $1`,
			userID,
		)

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	const (
		reservationCount = 20
		batchSize        = 5
	)

	// 1. Create storage record.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage
			(user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create storage record: %v", err)
	}

	// 2. Create 20 individual reservations.
	var totalReserved int64

	for i := 1; i <= reservationCount; i++ {
		size := int64(i * 100)

		_, err := db.CreateStorageReservation(ctx, userID, size)
		if err != nil {
			t.Fatalf("create reservation %d: %v", i, err)
		}

		totalReserved += size
	}

	// 3. Expire all pending reservations.
	_, err = pool.Exec(ctx, `
		UPDATE storage_reservations
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second',
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
		  AND status = 'PENDING'
	`, userID)
	if err != nil {
		t.Fatalf("expire reservations: %v", err)
	}

	// Confirm initial reserved quota.
	var reservedBefore int64

	err = pool.QueryRow(ctx, `
		SELECT reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&reservedBefore)
	if err != nil {
		t.Fatalf("read initial reserved quota: %v", err)
	}

	if reservedBefore != totalReserved {
		t.Fatalf(
			"initial reserved_bytes = %d; want %d",
			reservedBefore,
			totalReserved,
		)
	}

	// 4. Simulate two independent recovery workers.
	type workerResult struct {
		workerID  int
		recovered int
		err       error
	}

	results := make(chan workerResult, 2)
	start := make(chan struct{})

	runWorker := func(workerID int) {
		// Both workers start together.
		<-start

		totalRecovered := 0

		for {
			recovered, err := db.RecoverExpiredStorageReservations(
				ctx,
				batchSize,
			)
			if err != nil {
				results <- workerResult{
					workerID: workerID,
					err:      err,
				}
				return
			}

			totalRecovered += recovered

			// Continue processing while a full batch was recovered.
			if recovered < batchSize {
				break
			}
		}

		results <- workerResult{
			workerID:  workerID,
			recovered: totalRecovered,
		}
	}

	go runWorker(1)
	go runWorker(2)

	// Release both workers simultaneously.
	close(start)

	totalRecovered := 0

	for i := 0; i < 2; i++ {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf(
					"worker %d failed: %v",
					result.workerID,
					result.err,
				)
			}

			totalRecovered += result.recovered

			t.Logf(
				"Worker %d recovered %d reservations",
				result.workerID,
				result.recovered,
			)

		case <-ctx.Done():
			t.Fatal("timed out waiting for recovery workers")
		}
	}

	// 5. Verify total recovery count.
	if totalRecovered != reservationCount {
		t.Fatalf(
			"total recovered = %d; want %d",
			totalRecovered,
			reservationCount,
		)
	}

	// 6. Verify all reservations have been released exactly once.
	var pendingCount, releasedCount, finalizedCount int

	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'PENDING'),
			COUNT(*) FILTER (WHERE status = 'RELEASED'),
			COUNT(*) FILTER (WHERE status = 'FINALIZED')
		FROM storage_reservations
		WHERE user_id = $1
	`, userID).Scan(
		&pendingCount,
		&releasedCount,
		&finalizedCount,
	)
	if err != nil {
		t.Fatalf("read reservation statuses: %v", err)
	}

	if pendingCount != 0 {
		t.Errorf("pending reservations = %d; want 0", pendingCount)
	}

	if releasedCount != reservationCount {
		t.Errorf(
			"released reservations = %d; want %d",
			releasedCount,
			reservationCount,
		)
	}

	if finalizedCount != 0 {
		t.Errorf("finalized reservations = %d; want 0", finalizedCount)
	}

	// 7. Verify final quota accounting.
	var usedBytes, reservedBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read final storage usage: %v", err)
	}

	if usedBytes != 0 {
		t.Errorf("used_bytes = %d; want 0", usedBytes)
	}

	if reservedBytes != 0 {
		t.Errorf("reserved_bytes = %d; want 0", reservedBytes)
	}

	t.Logf(
		"PASS: two workers recovered %d reservations; released=%d, used=%d, reserved=%d",
		totalRecovered,
		releasedCount,
		usedBytes,
		reservedBytes,
	)
}
