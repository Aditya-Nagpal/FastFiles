package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	workerconfig "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/config"
	workerdb "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
)

func TestS3CleanupConsumer_FileDeleted_Integration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	envPath, err := findWorkerEnvFile()
	if err != nil {
		t.Fatalf("failed to find .env: %v", err)
	}
	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	brokersValue := os.Getenv("KAFKA_BROKERS")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	const testBucket = "fastfiles-bucket-test"

	if databaseURL == "" || brokersValue == "" {
		t.Fatal("DATABASE_URL and KAFKA_BROKERS must be configured")
	}
	if accessKey == "" || secretKey == "" || region == "" {
		t.Fatal("AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION must be configured")
	}

	brokers := strings.Split(brokersValue, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	previousDB := workerdb.DB
	workerdb.DB = pool

	previousConfig := workerconfig.AppConfig
	workerconfig.AppConfig = &workerconfig.Config{
		DatabaseURL:            databaseURL,
		BucketName:             testBucket,
		AWSAccessKeyId:         accessKey,
		AWSSecretAccessKey:     secretKey,
		AWSRegion:              region,
		KafkaBrokers:           brokersValue,
		KafkaS3CleanupDLQTopic: os.Getenv("KAFKA_S3_CLEANUP_DLQ_TOPIC"),
	}

	t.Cleanup(func() {
		workerdb.DB = previousDB
		workerconfig.AppConfig = previousConfig
		pool.Close()
	})

	// Configure the worker's actual global S3 uploader
	// to point to the dedicated integration-test bucket.
	utils.NewS3Uploader()

	awsConfig := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}
	s3Client := s3.NewFromConfig(awsConfig)

	testID := uuid.NewString()
	userID := time.Now().UnixNano()
	publicID := strings.ReplaceAll(testID, "-", "")[:21]
	fileSize := int64(128)

	s3Key := fmt.Sprintf(
		"integration-tests/%s/test-file.txt",
		testID,
	)

	// Upload a real object to the dedicated test bucket.
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(s3Key),
		Body:   strings.NewReader("FastFiles integration test object"),
	})
	if err != nil {
		t.Fatalf("failed to upload test S3 object: %v", err)
	}

	// Create the entry that the worker will resolve after deletion.
	var entryID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id,
			user_id,
			parent_id,
			name,
			type,
			content_type,
			extension,
			size,
			s3_key
		)
		VALUES ($1, $2, NULL, $3, 'FILE', $4, $5, $6, $7)
		RETURNING id
	`,
		publicID,
		userID,
		"test-file",
		"text/plain",
		"txt",
		fileSize,
		s3Key,
	).Scan(&entryID)

	if err != nil {
		t.Fatalf("failed to create worker DB entry: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, _ = s3Client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(s3Key),
		})

		_, err := pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE id = $1 AND user_id = $2`,
			entryID,
			userID,
		)
		if err != nil {
			t.Errorf("failed to clean worker DB entry: %v", err)
		}
	})

	// Create an isolated topic and consumer group.
	topic := "fastfiles-s3-cleanup-" + testID
	groupID := "s3-cleanup-integration-" + testID

	createWorkerKafkaTopic(t, brokers, topic)

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- runS3CleanupConsumer(
			consumerCtx,
			brokers,
			topic,
			groupID,
		)
	}()

	// Give the consumer time to join the group before publishing.
	time.Sleep(2 * time.Second)

	event := events.EntryEvent{
		EventID:     uuid.NewString(),
		EventType:   events.EventFileDeleted,
		AggregateID: publicID,
		UserID:      userID,
		EntryID:     entryID,
		PublicID:    publicID,
		EntryType:   "FILE",
		Size:        fileSize,
		CreatedAt:   time.Now().UTC(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		stopConsumer()
		t.Fatalf("failed to marshal event: %v", err)
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}

	defer writer.Close()

	publish := func() {
		t.Helper()

		publishCtx, publishCancel := context.WithTimeout(ctx, 10*time.Second)
		defer publishCancel()

		err := writer.WriteMessages(publishCtx, kafka.Message{
			Key:   []byte(fmt.Sprintf("%d", userID)),
			Value: payload,
			Headers: []kafka.Header{
				{Key: "event_id", Value: []byte(event.EventID)},
				{Key: "event_type", Value: []byte(event.EventType)},
			},
		})
		if err != nil {
			t.Fatalf("failed to publish cleanup event: %v", err)
		}
	}

	// Publish the FILE_DELETED event.
	publish()

	// Wait until the real consumer deletes the S3 object.
	waitForS3ObjectDeletion(t, ctx, s3Client, testBucket, s3Key)

	t.Log("SUCCESS: Kafka consumer processed FILE_DELETED and deleted the S3 object")

	// Deliver the same event again to verify idempotent cleanup.
	publish()

	// The handler should also safely process the duplicate.
	// The object is already absent; DeleteObject must remain successful.
	if err := utils.DeleteS3ObjectWithClient(
		ctx,
		s3Client,
		testBucket,
		s3Key,
	); err != nil {
		t.Fatalf("duplicate S3 deletion was not safe: %v", err)
	}

	waitForS3ObjectDeletion(t, ctx, s3Client, testBucket, s3Key)

	t.Log("SUCCESS: repeated S3 deletion is safe")

	stopConsumer()

	select {
	case err := <-consumerDone:
		if err != nil {
			t.Errorf("consumer returned unexpected error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Error("consumer did not stop within 15 seconds")
	}
}

func waitForS3ObjectDeletion(
	t *testing.T,
	ctx context.Context,
	client *s3.Client,
	bucket string,
	key string,
) {
	t.Helper()

	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})

		if err != nil {
			var responseErr interface {
				ErrorCode() string
			}

			if errors.As(err, &responseErr) {
				code := responseErr.ErrorCode()
				if code == "NotFound" || code == "NoSuchKey" {
					return
				}
			}
		} else {
			// Object still exists. Keep waiting.
		}

		select {
		case <-ctx.Done():
			t.Fatalf("S3 object was not deleted before timeout: %s", key)
		case <-ticker.C:
		}
	}
}

func findWorkerEnvFile() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for i := 0; i < 5; i++ {
		candidate := currentDir + string(os.PathSeparator) + ".env"
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := currentDir[:strings.LastIndex(currentDir, string(os.PathSeparator))]
		if parent == currentDir || parent == "" {
			break
		}
		currentDir = parent
	}

	return "", fmt.Errorf(".env file not found")
}

func createWorkerKafkaTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()

	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("failed to connect to Kafka: %v", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("failed to get Kafka controller: %v", err)
	}

	controllerConn, err := kafka.Dial(
		"tcp",
		fmt.Sprintf("%s:%d", controller.Host, controller.Port),
	)
	if err != nil {
		t.Fatalf("failed to connect to Kafka controller: %v", err)
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil {
		t.Fatalf("failed to create Kafka topic %s: %v", topic, err)
	}
}

func TestS3CleanupConsumer_FolderDeleted_Integration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	envPath, err := findWorkerEnvFile()
	if err != nil {
		t.Fatalf("failed to find .env: %v", err)
	}
	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	brokersValue := os.Getenv("KAFKA_BROKERS")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")

	const testBucket = "fastfiles-bucket-test"

	if databaseURL == "" || brokersValue == "" {
		t.Fatal("DATABASE_URL and KAFKA_BROKERS must be configured")
	}
	if accessKey == "" || secretKey == "" || region == "" {
		t.Fatal("AWS credentials and AWS_REGION must be configured")
	}

	brokers := strings.Split(brokersValue, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	previousDB := workerdb.DB
	previousConfig := workerconfig.AppConfig

	workerdb.DB = pool
	workerconfig.AppConfig = &workerconfig.Config{
		DatabaseURL:            databaseURL,
		BucketName:             testBucket,
		AWSAccessKeyId:         accessKey,
		AWSSecretAccessKey:     secretKey,
		AWSRegion:              region,
		KafkaBrokers:           brokersValue,
		KafkaS3CleanupDLQTopic: os.Getenv("KAFKA_S3_CLEANUP_DLQ_TOPIC"),
	}

	t.Cleanup(func() {
		workerdb.DB = previousDB
		workerconfig.AppConfig = previousConfig
		pool.Close()
	})

	utils.NewS3Uploader()

	s3Client := s3.NewFromConfig(aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	})

	testID := uuid.NewString()
	userID := time.Now().UnixNano()

	rootPublicID := strings.ReplaceAll(uuid.NewString(), "-", "")[:21]
	nestedPublicID := strings.ReplaceAll(uuid.NewString(), "-", "")[:21]

	prefix := "integration-tests/" + testID + "/"

	type testFile struct {
		name string
		key  string
	}

	files := []testFile{
		{
			name: "file-a.txt",
			key:  prefix + "file-a.txt",
		},
		{
			name: "file-b.txt",
			key:  prefix + "nested/file-b.txt",
		},
		{
			name: "file-c.txt",
			key:  prefix + "nested/file-c.txt",
		},
	}

	// Upload three real objects into the dedicated test bucket.
	for _, file := range files {
		_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(testBucket),
			Key:    aws.String(file.key),
			Body:   strings.NewReader("FastFiles folder cleanup integration test"),
		})
		if err != nil {
			t.Fatalf("failed to upload %s: %v", file.name, err)
		}
	}

	// Create the root folder.
	var rootID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id, user_id, parent_id, name, type,
			content_type, extension, size, s3_key
		)
		VALUES ($1, $2, NULL, 'root-folder', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)

	if err != nil {
		t.Fatalf("failed to create root folder: %v", err)
	}

	// Create a nested folder.
	var nestedID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id, user_id, parent_id, name, type,
			content_type, extension, size, s3_key
		)
		VALUES ($1, $2, $3, 'nested-folder', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, nestedPublicID, userID, rootID).Scan(&nestedID)

	if err != nil {
		t.Fatalf("failed to create nested folder: %v", err)
	}

	// Create file A directly under root.
	var fileAID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id, user_id, parent_id, name, type,
			content_type, extension, size, s3_key
		)
		VALUES ($1, $2, $3, 'file-a', 'FILE', 'text/plain', 'txt', 128, $4)
		RETURNING id
	`,
		strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
		userID,
		rootID,
		files[0].key,
	).Scan(&fileAID)

	if err != nil {
		t.Fatalf("failed to create file A: %v", err)
	}

	// Create files B and C under nested folder.
	var fileBID, fileCID int64

	for i, file := range files[1:] {
		var id int64
		err = pool.QueryRow(ctx, `
			INSERT INTO entries (
				public_id, user_id, parent_id, name, type,
				content_type, extension, size, s3_key
			)
			VALUES ($1, $2, $3, $4, 'FILE', 'text/plain', 'txt', 128, $5)
			RETURNING id
		`,
			strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
			userID,
			nestedID,
			file.name,
			file.key,
		).Scan(&id)

		if err != nil {
			t.Fatalf("failed to create nested file %s: %v", file.name, err)
		}

		if i == 0 {
			fileBID = id
		} else {
			fileCID = id
		}
	}

	// Clean up only this test's database rows and S3 prefix.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
			  AND id = ANY($2::BIGINT[])
		`, userID, []int64{rootID, nestedID, fileAID, fileBID, fileCID})
		if err != nil {
			t.Errorf("failed to clean folder fixture: %v", err)
		}

		for _, file := range files {
			_, err := s3Client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{
				Bucket: aws.String(testBucket),
				Key:    aws.String(file.key),
			})
			if err != nil {
				t.Errorf("failed to clean S3 object %s: %v", file.key, err)
			}
		}
	})

	// Simulate the state after DeleteFolder has soft-deleted
	// the root folder and all its descendants.
	_, err = pool.Exec(ctx, `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP,
		    updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $1
		  AND id = ANY($2::BIGINT[])
	`, userID, []int64{rootID, nestedID, fileAID, fileBID, fileCID})

	if err != nil {
		t.Fatalf("failed to soft-delete folder hierarchy fixture: %v", err)
	}

	topic := "fastfiles-folder-cleanup-" + testID
	groupID := "folder-cleanup-integration-" + testID

	createWorkerKafkaTopic(t, brokers, topic)

	consumerCtx, stopConsumer := context.WithCancel(ctx)
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- runS3CleanupConsumer(
			consumerCtx,
			brokers,
			topic,
			groupID,
		)
	}()

	time.Sleep(2 * time.Second)

	event := events.EntryEvent{
		EventID:     uuid.NewString(),
		EventType:   events.EventFolderDeleted,
		AggregateID: rootPublicID,
		UserID:      userID,
		EntryID:     rootID,
		PublicID:    rootPublicID,
		EntryType:   "FOLDER",
		Size:        384,
		CreatedAt:   time.Now().UTC(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		stopConsumer()
		t.Fatalf("failed to marshal folder event: %v", err)
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	defer writer.Close()

	publishCtx, publishCancel := context.WithTimeout(ctx, 10*time.Second)
	err = writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(fmt.Sprintf("%d", userID)),
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(event.EventID)},
			{Key: "event_type", Value: []byte(event.EventType)},
		},
	})
	publishCancel()

	if err != nil {
		stopConsumer()
		t.Fatalf("failed to publish folder deletion event: %v", err)
	}

	// All three objects must be deleted, including nested descendants.
	for _, file := range files {
		waitForS3ObjectDeletion(t, ctx, s3Client, testBucket, file.key)
		t.Logf("SUCCESS: deleted S3 object %s", file.key)
	}

	stopConsumer()

	select {
	case err := <-consumerDone:
		if err != nil {
			t.Errorf("consumer returned unexpected error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Error("consumer did not stop within 15 seconds")
	}

	t.Log("SUCCESS: FOLDER_DELETED cleaned up all descendant S3 objects")
}
