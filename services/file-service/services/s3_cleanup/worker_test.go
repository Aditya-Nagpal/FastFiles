package s3_cleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
)

func newTestWorker(
	t *testing.T,
	tasks []*models.S3CleanupTask,
	deleteErr error,
) (cleanupWorker, *[]int64, *[]int64, *[]error) {
	t.Helper()

	var completedIDs []int64
	var failedIDs []int64
	var deletionErrors []error

	worker := cleanupWorker{
		claimTask: func(ctx context.Context) (*models.S3CleanupTask, error) {
			if len(tasks) == 0 {
				return nil, nil
			}

			task := tasks[0]
			tasks = tasks[1:]
			return task, nil
		},

		markDone: func(ctx context.Context, id int64) error {
			completedIDs = append(completedIDs, id)
			return nil
		},

		markFailed: func(ctx context.Context, id int64, err error) error {
			failedIDs = append(failedIDs, id)
			deletionErrors = append(deletionErrors, err)
			return nil
		},

		deleteS3: func(
			ctx context.Context,
			bucket string,
			key string,
		) error {
			return deleteErr
		},
	}

	return worker, &completedIDs, &failedIDs, &deletionErrors
}

func TestProcessTask_Success(t *testing.T) {
	task := &models.S3CleanupTask{
		ID:          1,
		BucketName:  "test-bucket",
		S3Key:       "user/file.txt",
		Attempts:    1,
		MaxAttempts: 10,
	}

	worker, completed, failed, _ := newTestWorker(
		t,
		nil,
		nil,
	)

	processTask(context.Background(), worker, task)

	if len(*completed) != 1 || (*completed)[0] != task.ID {
		t.Fatalf("expected task %d to be marked completed, got %v", task.ID, *completed)
	}

	if len(*failed) != 0 {
		t.Fatalf("expected no failed tasks, got %v", *failed)
	}
}

func TestProcessTask_DeleteFailure(t *testing.T) {
	expectedErr := errors.New("S3 deletion failed")

	task := &models.S3CleanupTask{
		ID:          2,
		BucketName:  "test-bucket",
		S3Key:       "user/file.txt",
		Attempts:    1,
		MaxAttempts: 10,
	}

	worker, completed, failed, deletionErrors := newTestWorker(
		t,
		nil,
		expectedErr,
	)

	processTask(context.Background(), worker, task)

	if len(*completed) != 0 {
		t.Fatalf("expected no completed tasks, got %v", *completed)
	}

	if len(*failed) != 1 || (*failed)[0] != task.ID {
		t.Fatalf("expected task %d to be marked failed, got %v", task.ID, *failed)
	}

	if len(*deletionErrors) != 1 || !errors.Is((*deletionErrors)[0], expectedErr) {
		t.Fatalf("expected original deletion error to be recorded")
	}
}

func TestProcessTask_CompletionUpdateFailure(t *testing.T) {
	task := &models.S3CleanupTask{
		ID:          3,
		BucketName:  "test-bucket",
		S3Key:       "user/file.txt",
		Attempts:    1,
		MaxAttempts: 10,
	}

	expectedErr := errors.New("database update failed")

	worker, _, failed, _ := newTestWorker(t, nil, nil)

	worker.markDone = func(ctx context.Context, id int64) error {
		return expectedErr
	}

	processTask(context.Background(), worker, task)

	if len(*failed) != 0 {
		t.Fatalf("task should not be marked as failed when completion update fails")
	}
}

func TestProcessAvailableTasks_MultipleTasks(t *testing.T) {
	tasks := []*models.S3CleanupTask{
		{
			ID:         10,
			BucketName: "test-bucket",
			S3Key:      "user/file1.txt",
		},
		{
			ID:         11,
			BucketName: "test-bucket",
			S3Key:      "user/file2.txt",
		},
	}

	worker, completed, failed, _ := newTestWorker(t, tasks, nil)

	processed := processAvailableTasks(context.Background(), worker)

	if !processed {
		t.Fatal("expected at least one task to be processed")
	}

	if len(*completed) != 2 {
		t.Fatalf("expected 2 completed tasks, got %d", len(*completed))
	}

	if len(*failed) != 0 {
		t.Fatalf("expected no failed tasks, got %v", *failed)
	}
}

func TestProcessAvailableTasks_EmptyQueue(t *testing.T) {
	worker, _, _, _ := newTestWorker(t, nil, nil)

	processed := processAvailableTasks(context.Background(), worker)

	if processed {
		t.Fatal("expected no tasks to be processed")
	}
}

func TestProcessAvailableTasks_ClaimFailure(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	worker, _, _, _ := newTestWorker(t, nil, nil)

	worker.claimTask = func(ctx context.Context) (*models.S3CleanupTask, error) {
		return nil, expectedErr
	}

	processed := processAvailableTasks(context.Background(), worker)

	if processed {
		t.Fatal("expected no tasks to be processed")
	}
}

func TestProcessTask_DeleteRespectsContext(t *testing.T) {
	task := &models.S3CleanupTask{
		ID:         20,
		BucketName: "test-bucket",
		S3Key:      "user/file.txt",
	}

	worker, completed, failed, _ := newTestWorker(t, nil, nil)

	worker.deleteS3 = func(
		ctx context.Context,
		bucket string,
		key string,
	) error {
		<-ctx.Done()
		return ctx.Err()
	}

	// A cancelled parent context should cancel the S3 operation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processTask(ctx, worker, task)

	if len(*completed) != 0 {
		t.Fatal("cancelled deletion must not be marked completed")
	}

	if len(*failed) != 1 {
		t.Fatal("expected cancelled deletion to be recorded as failed")
	}
}

func TestRunWith_StopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	worker, _, _, _ := newTestWorker(t, nil, nil)

	done := make(chan struct{})

	go func() {
		defer close(done)
		runWith(ctx, 10*time.Millisecond, worker)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}
