package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"os"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

const s3CompensationTestBucket = "fastfiles-bucket-test"

type aclIgnoringS3Client struct {
	utils.S3PutObjectAPI
}

func (c *aclIgnoringS3Client) PutObject(
	ctx context.Context,
	input *s3.PutObjectInput,
	optFns ...func(*s3.Options),
) (*s3.PutObjectOutput, error) {
	clonedInput := *input
	clonedInput.ACL = ""

	return c.S3PutObjectAPI.PutObject(
		ctx,
		&clonedInput,
		optFns...,
	)
}

func TestCleanupUploadedS3ObjectAfterParentDeletion(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	if accessKey == "" || secretKey == "" || region == "" {
		t.Skip("AWS credentials or region are not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	previousDB := db.DB
	db.DB = pool

	previousConfig := config.AppConfig
	config.AppConfig = &config.Config{
		AWSRegion:          region,
		AWSAccessKeyId:     accessKey,
		AWSSecretAccessKey: secretKey,
		BucketName:         s3CompensationTestBucket,
	}

	var testUserID int64
	testUserID = time.Now().UnixNano()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM s3_cleanup_tasks
			WHERE s3_key LIKE $1
		`, fmt.Sprintf("integration-tests/%s/%%", t.Name()))

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, testUserID)

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM storage_reservations
			WHERE user_id = $1
		`, testUserID)

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, testUserID)

		db.DB = previousDB
		config.AppConfig = previousConfig
		pool.Close()
	})

	uploader, err := utils.NewS3Uploader()
	if err != nil {
		t.Fatalf("initialize S3 uploader: %v", err)
	}

	uploader.PutObjectClient = &aclIgnoringS3Client{
		S3PutObjectAPI: uploader.Client,
	}

	// Unique test identifiers.
	testUserID = time.Now().UnixNano()

	publicFolderID := fmt.Sprintf("p%d", time.Now().UnixNano())
	publicFileID := fmt.Sprintf("f%d", time.Now().UnixNano())

	s3Key := fmt.Sprintf(
		"integration-tests/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	// 1. Create user storage.
	_, err = db.DB.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, 1000000, 0, 0)
	`, testUserID)

	if err != nil {
		t.Fatalf("create user storage: %v", err)
	}

	// 2. Create parent folder.
	var parentID int64

	err = db.DB.QueryRow(ctx, `
		INSERT INTO entries (
			public_id,
			user_id,
			parent_id,
			name,
			type,
			extension,
			size
		)
		VALUES ($1, $2, NULL, 'test-folder', 'FOLDER', '', 0)
		RETURNING id
	`, publicFolderID, testUserID).Scan(&parentID)

	if err != nil {
		t.Fatalf("create parent folder: %v", err)
	}

	// 3. Create a real temporary file.
	content := []byte("FastFiles S3 compensation integration test")

	tempFile, err := os.CreateTemp(t.TempDir(), "compensation-*.txt")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tempFile.Write(content); err != nil {
		tempFile.Close()
		t.Fatal(err)
	}

	if _, err := tempFile.Seek(0, 0); err != nil {
		tempFile.Close()
		t.Fatal(err)
	}

	defer tempFile.Close()

	header := &multipart.FileHeader{
		Filename: "compensation.txt",
		Size:     int64(len(content)),
		Header:   make(textproto.MIMEHeader),
	}

	header.Header.Set("Content-Type", "text/plain")

	// 4. Reserve storage before uploading.
	reservationID, err := db.CreateStorageReservation(
		ctx,
		testUserID,
		int64(len(content)),
	)

	if err != nil {
		t.Fatalf("create storage reservation: %v", err)
	}

	// 5. Upload an actual object to the test S3 bucket.
	if err := uploader.UploadFile(ctx, tempFile, header, s3Key); err != nil {
		t.Fatalf("upload test object to S3: %v", err)
	}

	// Always attempt to remove the test object if the test exits early.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			20*time.Second,
		)
		defer cleanupCancel()

		_ = uploader.DeleteObjectWithContext(
			cleanupCtx,
			s3CompensationTestBucket,
			s3Key,
		)
	}()

	// 6. Simulate parent folder deletion while upload is in progress.
	_, err = db.DB.Exec(ctx, `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, parentID)

	if err != nil {
		t.Fatalf("simulate parent folder deletion: %v", err)
	}

	// 7. Attempt to finalize the upload.
	parentIDCopy := parentID

	entryData := &models.EntryData{
		PublicId:    publicFileID,
		UserId:      testUserID,
		ParentId:    &parentIDCopy,
		Name:        "compensation",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   ".txt",
		Size:        int64(len(content)),
		S3Key:       sql.NullString{String: s3Key, Valid: true},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = db.InsertEntryAndFinalizeStorage(
		ctx,
		entryData,
		reservationID,
	)

	if !errors.Is(err, db.ErrEntryNotFound) {
		t.Fatalf(
			"expected ErrEntryNotFound after parent deletion, got: %v",
			err,
		)
	}

	// 8. Run the same S3 compensation function used by the upload handler.
	CleanupUploadedS3Object(uploader, s3Key)

	// 9. Simulate the upload handler's deferred reservation release.
	if err := db.ReleaseStorageReservationByID(
		ctx,
		reservationID,
	); err != nil {
		t.Fatalf("release storage reservation: %v", err)
	}

	// 10. Verify S3 object is actually gone.
	_, err = uploader.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s3CompensationTestBucket),
		Key:    aws.String(s3Key),
	})

	if err == nil {
		t.Fatal("S3 object still exists after compensation")
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) ||
		(apiErr.ErrorCode() != "NotFound" &&
			apiErr.ErrorCode() != "NoSuchKey") {
		t.Fatalf("expected S3 NotFound error, got: %v", err)
	}

	// 11. Verify cleanup task completed.
	var cleanupStatus string

	err = db.DB.QueryRow(ctx, `
		SELECT status
		FROM s3_cleanup_tasks
		WHERE bucket_name = $1 AND s3_key = $2
		ORDER BY id DESC
		LIMIT 1
	`, s3CompensationTestBucket, s3Key).Scan(&cleanupStatus)

	if err != nil {
		t.Fatalf("fetch cleanup task: %v", err)
	}

	if cleanupStatus != "COMPLETED" {
		t.Fatalf(
			"cleanup status = %s; want COMPLETED",
			cleanupStatus,
		)
	}

	// 12. Verify reservation was released and quota is unchanged.
	var reservationStatus string

	err = db.DB.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE id = $1
	`, reservationID).Scan(&reservationStatus)

	if err != nil {
		t.Fatalf("fetch reservation status: %v", err)
	}

	if reservationStatus != "RELEASED" {
		t.Fatalf(
			"reservation status = %s; want RELEASED",
			reservationStatus,
		)
	}

	var usedBytes, reservedBytes int64

	err = db.DB.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, testUserID).Scan(&usedBytes, &reservedBytes)

	if err != nil {
		t.Fatalf("fetch storage usage: %v", err)
	}

	if usedBytes != 0 || reservedBytes != 0 {
		t.Fatalf(
			"storage accounting mismatch: used=%d reserved=%d; want both 0",
			usedBytes,
			reservedBytes,
		)
	}

	// 13. Verify no file entry was inserted.
	var fileCount int

	err = db.DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1
		  AND public_id = $2
	`, testUserID, publicFileID).Scan(&fileCount)

	if err != nil {
		t.Fatal(err)
	}

	if fileCount != 0 {
		t.Fatalf("file entry count = %d; want 0", fileCount)
	}

	t.Log("PASS: uploaded S3 object compensated successfully after parent deletion")
}
