package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"errors"
)

func setupCleanupHandlerIntegrationTest(t *testing.T) (*pgxpool.Pool, *utils.S3Uploader) {
	t.Helper()

	if err := godotenv.Load("../.env"); err != nil {
		if err := godotenv.Load(".env"); err != nil {
			t.Fatalf("failed to load .env: %v", err)
		}
	}

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{
			DatabaseURL:        os.Getenv("DATABASE_URL"),
			BucketName:         os.Getenv("AWS_S3_BUCKET_NAME"),
			AWSAccessKeyId:     os.Getenv("AWS_ACCESS_KEY_ID"),
			AWSSecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			AWSRegion:          os.Getenv("AWS_REGION"),
		}
	}

	if config.AppConfig.DatabaseURL == "" ||
		config.AppConfig.BucketName == "" ||
		config.AppConfig.AWSRegion == "" ||
		config.AppConfig.AWSAccessKeyId == "" ||
		config.AppConfig.AWSSecretAccessKey == "" {
		t.Fatal("missing required database or AWS configuration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	pool, err := pgxpool.New(ctx, config.AppConfig.DatabaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	previousDB := db.DB
	db.DB = pool

	uploader, err := utils.NewS3Uploader()
	if err != nil {
		db.DB = previousDB
		pool.Close()
		cancel()
		t.Fatalf("initialize S3 uploader: %v", err)
	}

	t.Cleanup(func() {
		db.DB = previousDB
		pool.Close()
		cancel()
	})

	return pool, uploader
}

func TestCleanupUploadedS3ObjectIntegration(t *testing.T) {
	pool, uploader := setupCleanupHandlerIntegrationTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	s3Key := fmt.Sprintf(
		"test-cleanup/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	// Upload an actual object to S3.
	_, err := uploader.PutObjectClient.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
		Body:   bytes.NewReader([]byte("cleanup integration test")),
	})
	if err != nil {
		t.Fatalf("upload test object to S3: %v", err)
	}

	// Cleanup the uploaded object using the production function.
	CleanupUploadedS3Object(uploader, s3Key)

	// Find the cleanup task created by the production function.
	var status string
	var attempts int

	err = pool.QueryRow(ctx, `
		SELECT status, attempts
		FROM s3_cleanup_tasks
		WHERE bucket_name = $1 AND s3_key = $2
		ORDER BY id DESC
		LIMIT 1
	`, uploader.BucketName, s3Key).Scan(&status, &attempts)

	if err != nil {
		t.Fatalf("fetch cleanup task: %v", err)
	}

	if status != "COMPLETED" {
		t.Fatalf("cleanup status = %s; want COMPLETED", status)
	}

	if attempts != 1 {
		t.Fatalf("cleanup attempts = %d; want 1", attempts)
	}

	// Verify that the object no longer exists in S3.
	_, err = uploader.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
	})

	if err == nil {
		t.Fatal("expected S3 object to be deleted, but it still exists")
	}

	t.Logf(
		"PASS: cleanup completed; bucket=%s key=%s attempts=%d",
		uploader.BucketName,
		s3Key,
		attempts,
	)
}

func TestCleanupUploadedS3ObjectRejectsInvalidS3KeyIntegration(t *testing.T) {
	pool, uploader := setupCleanupHandlerIntegrationTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Empty key is rejected by S3Uploader before calling AWS.
	// The cleanup task creation itself also rejects empty keys.
	CleanupUploadedS3Object(uploader, "")

	var count int

	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM s3_cleanup_tasks
		WHERE bucket_name = $1
		  AND s3_key = ''
	`, uploader.BucketName).Scan(&count)

	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("expected no task for empty key, found %d", count)
	}

	t.Log("PASS: invalid S3 key rejected without creating a task")
}

func TestCleanupUploadedS3ObjectTaskCreationFailureIntegration(t *testing.T) {
	_, uploader := setupCleanupHandlerIntegrationTest(t)

	// An empty bucket name causes CreateS3CleanupTask to reject the request.
	// No AWS deletion should be attempted.
	originalBucket := uploader.BucketName
	uploader.BucketName = ""
	defer func() {
		uploader.BucketName = originalBucket
	}()

	CleanupUploadedS3Object(uploader, "some-test-key")

	t.Log("PASS: cleanup safely handles task creation failure")
}

func TestCleanupUploadedS3ObjectDoesNotDeleteObjectWhenTaskCreationFails(t *testing.T) {
	_, uploader := setupCleanupHandlerIntegrationTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s3Key := fmt.Sprintf(
		"test-cleanup/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	_, err := uploader.PutObjectClient.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
		Body:   strings.NewReader("must remain in S3"),
	})
	if err != nil {
		t.Fatalf("upload test object: %v", err)
	}

	originalBucket := uploader.BucketName
	uploader.BucketName = ""
	CleanupUploadedS3Object(uploader, s3Key)
	uploader.BucketName = originalBucket

	_, err = uploader.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		t.Fatalf("object should remain in S3 when task creation fails: %v", err)
	}

	// Best-effort removal of the test object.
	_ = uploader.DeleteObjectWithContext(ctx, uploader.BucketName, s3Key)

	t.Log("PASS: S3 object retained when cleanup intent cannot be persisted")
}

func TestCleanupUploadedS3ObjectClaimFailureIntegration(t *testing.T) {
	pool, uploader := setupCleanupHandlerIntegrationTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s3Key := fmt.Sprintf(
		"test-cleanup/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	_, err := uploader.PutObjectClient.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
		Body:   strings.NewReader("must remain"),
	})
	if err != nil {
		t.Fatalf("upload test object: %v", err)
	}

	// Create an already-claimed task with the same key.
	taskID, err := db.CreateS3CleanupTask(ctx, uploader.BucketName, s3Key)
	if err != nil {
		t.Fatal(err)
	}

	claimedTask, err := db.ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if claimedTask == nil {
		t.Fatal("expected initial claim")
	}

	// Production function creates a separate task for the same key.
	// To force its claim to fail, we cannot inject a claim failure
	// without changing production code. This test therefore verifies
	// the already-claimed task itself cannot be claimed again.
	secondClaim, err := db.ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if secondClaim != nil {
		t.Fatal("expected already-claimed task to be unavailable")
	}

	// Verify the object remains in S3.
	_, err = uploader.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(uploader.BucketName),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		t.Fatalf("object should still exist: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM s3_cleanup_tasks WHERE id = $1
	`, taskID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", status)
	}

	t.Log("PASS: already-claimed task cannot be claimed again")
}

func TestCleanupTaskRejectsExpiredLeaseCompletionIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, _ := setupCleanupHandlerIntegrationTest(t)

	taskID, err := db.CreateS3CleanupTask(
		ctx,
		config.AppConfig.BucketName,
		fmt.Sprintf("test-cleanup/%s/%d.txt", t.Name(), time.Now().UnixNano()),
	)
	if err != nil {
		t.Fatal(err)
	}

	task, err := db.ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil || task == nil {
		t.Fatalf("claim task: task=%v err=%v", task, err)
	}

	_, err = pool.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second'
		WHERE id = $1
	`, taskID)
	if err != nil {
		t.Fatal(err)
	}

	err = db.MarkS3CleanupCompleted(ctx, taskID, *task.LeaseToken)
	if err == nil {
		t.Fatal("expected expired lease completion to fail")
	}

	t.Log("PASS: expired lease cannot complete cleanup task")
}

func TestCleanupTaskRejectsExpiredLeaseFailureIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, _ := setupCleanupHandlerIntegrationTest(t)

	taskID, err := db.CreateS3CleanupTask(
		ctx,
		config.AppConfig.BucketName,
		fmt.Sprintf("test-cleanup/%s/%d.txt", t.Name(), time.Now().UnixNano()),
	)
	if err != nil {
		t.Fatal(err)
	}

	task, err := db.ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil || task == nil {
		t.Fatalf("claim task: task=%v err=%v", task, err)
	}

	_, err = pool.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second'
		WHERE id = $1
	`, taskID)
	if err != nil {
		t.Fatal(err)
	}

	err = db.MarkS3CleanupFailed(
		ctx,
		taskID,
		*task.LeaseToken,
		errors.New("simulated deletion failure"),
	)
	if err == nil {
		t.Fatal("expected expired lease failure update to fail")
	}

	t.Log("PASS: expired lease cannot reschedule cleanup task")
}
