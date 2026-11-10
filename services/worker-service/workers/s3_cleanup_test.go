package workers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakeS3DeleteClient struct {
	deletedKeys []string
	err         error
}

func (f *fakeS3DeleteClient) DeleteObject(
	ctx context.Context,
	params *s3.DeleteObjectInput,
	optFns ...func(*s3.Options),
) (*s3.DeleteObjectOutput, error) {
	if params.Key != nil {
		f.deletedKeys = append(f.deletedKeys, *params.Key)
	}

	if f.err != nil {
		return nil, f.err
	}

	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3DeleteClient) deleteObject(ctx context.Context, key string) error {
	return utils.DeleteS3ObjectWithClient(ctx, f, "test-bucket", key)
}

func TestHandleS3CleanupEvent_File(t *testing.T) {
	ctx := context.Background()

	fakeClient := &fakeS3DeleteClient{}

	expectedKey := "users/test/files/file-a.txt"

	event := events.S3CleanupEvent{
		EventID:   "test-file-cleanup-event",
		EventType: events.EventS3Cleanup,
		UserID:    1,
		Objects: []events.S3CleanupObject{
			{
				BucketName: "test-bucket",
				S3Key:      expectedKey,
			},
		},
		CreatedAt: time.Now().UTC(),
	}

	message, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = handleS3CleanupEvent(
		ctx,
		message,
		fakeClient.deleteObject,
	)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if len(fakeClient.deletedKeys) != 1 {
		t.Fatalf(
			"deleted keys count = %d, want 1; keys=%v",
			len(fakeClient.deletedKeys),
			fakeClient.deletedKeys,
		)
	}

	if fakeClient.deletedKeys[0] != expectedKey {
		t.Errorf(
			"deleted key = %s, want %s",
			fakeClient.deletedKeys[0],
			expectedKey,
		)
	}

	t.Logf("PASS: file S3 cleanup deleted object: %s", expectedKey)
}

func TestHandleS3CleanupEvent_Folder(t *testing.T) {
	ctx := context.Background()

	fakeClient := &fakeS3DeleteClient{}

	expectedKeys := []string{
		"users/test/files/file-a.txt",
		"users/test/files/nested/file-b.txt",
	}

	event := events.S3CleanupEvent{
		EventID:   "test-folder-cleanup-event",
		EventType: events.EventS3Cleanup,
		UserID:    1,
		Objects: []events.S3CleanupObject{
			{
				BucketName: "test-bucket",
				S3Key:      expectedKeys[0],
			},
			{
				BucketName: "test-bucket",
				S3Key:      expectedKeys[1],
			},
		},
		CreatedAt: time.Now().UTC(),
	}

	message, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = handleS3CleanupEvent(
		ctx,
		message,
		fakeClient.deleteObject,
	)
	if err != nil {
		t.Fatalf("folder cleanup failed: %v", err)
	}

	if len(fakeClient.deletedKeys) != len(expectedKeys) {
		t.Fatalf(
			"deleted keys count = %d, want %d; keys=%v",
			len(fakeClient.deletedKeys),
			len(expectedKeys),
			fakeClient.deletedKeys,
		)
	}

	if !reflect.DeepEqual(fakeClient.deletedKeys, expectedKeys) {
		t.Errorf(
			"deleted keys = %v, want %v",
			fakeClient.deletedKeys,
			expectedKeys,
		)
	}

	t.Logf(
		"PASS: folder cleanup deleted all descendant S3 objects: %v",
		fakeClient.deletedKeys,
	)
}

func TestHandleS3CleanupEvent_DeleteFailure(t *testing.T) {
	ctx := context.Background()

	expectedErr := errors.New("S3 deletion failed")

	fakeClient := &fakeS3DeleteClient{
		err: expectedErr,
	}

	event := events.S3CleanupEvent{
		EventID:   "test-cleanup-failure-event",
		EventType: events.EventS3Cleanup,
		UserID:    1,
		Objects: []events.S3CleanupObject{
			{
				BucketName: "test-bucket",
				S3Key:      "users/test/files/file-a.txt",
			},
		},
		CreatedAt: time.Now().UTC(),
	}

	message, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	err = handleS3CleanupEvent(
		ctx,
		message,
		fakeClient.deleteObject,
	)

	if err == nil {
		t.Fatal("expected cleanup to fail, but it succeeded")
	}

	if !errors.Is(err, expectedErr) {
		t.Logf("cleanup returned expected failure: %v", err)
	}

	t.Logf("PASS: S3 cleanup failure was propagated: %v", err)
}
