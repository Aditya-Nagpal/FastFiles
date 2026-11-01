package s3_cleanup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
)

func newTestCleanupTask(id int64) *models.S3CleanupTask {
	token := uuid.New()

	return &models.S3CleanupTask{
		ID:             id,
		BucketName:     "test-bucket",
		S3Key:          "user/file.txt",
		Attempts:       1,
		MaxAttempts:    10,
		LeaseToken:     &token,
		LeaseExpiresAt: timePtr(time.Now().Add(2 * time.Minute)),
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func newTestWorker(
	t *testing.T,
	tasks []*models.S3CleanupTask,
	deleteErr error,
) (cleanupWorker, *[]int64, *[]int64, *[]error) {
	t.Helper()

	var completedIDs []int64
	var failedIDs []int64
	var deletionErrors []error

	// Every claimed test task must have a lease token.
	for _, task := range tasks {
		if task != nil && task.LeaseToken == nil {
			token := uuid.New()
			task.LeaseToken = &token
		}
	}

	worker := cleanupWorker{
		claimTask: func(ctx context.Context) (*models.S3CleanupTask, error) {
			if len(tasks) == 0 {
				return nil, nil
			}

			task := tasks[0]
			tasks = tasks[1:]
			return task, nil
		},

		markDone: func(
			ctx context.Context,
			id int64,
			token uuid.UUID,
		) error {
			if token == uuid.Nil {
				return errors.New("empty lease token passed to markDone")
			}

			completedIDs = append(completedIDs, id)
			return nil
		},

		markFailed: func(
			ctx context.Context,
			id int64,
			token uuid.UUID,
			err error,
		) error {
			if token == uuid.Nil {
				return errors.New("empty lease token passed to markFailed")
			}

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
	task := newTestCleanupTask(1)

	worker, completed, failed, _ := newTestWorker(
		t,
		nil,
		nil,
	)

	processTask(context.Background(), worker, task)

	if len(*completed) != 1 || (*completed)[0] != task.ID {
		t.Fatalf(
			"expected task %d to be marked completed, got %v",
			task.ID,
			*completed,
		)
	}

	if len(*failed) != 0 {
		t.Fatalf("expected no failed tasks, got %v", *failed)
	}
}

func TestProcessTask_DeleteFailure(t *testing.T) {
	expectedErr := errors.New("S3 deletion failed")
	task := newTestCleanupTask(2)

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
		t.Fatalf(
			"expected task %d to be marked failed, got %v",
			task.ID,
			*failed,
		)
	}

	if len(*deletionErrors) != 1 ||
		!errors.Is((*deletionErrors)[0], expectedErr) {
		t.Fatal("expected original deletion error to be recorded")
	}
}

func TestProcessTask_CompletionUpdateFailure(t *testing.T) {
	task := newTestCleanupTask(3)

	expectedErr := errors.New("database update failed")

	worker, _, failed, _ := newTestWorker(t, nil, nil)

	worker.markDone = func(
		ctx context.Context,
		id int64,
		token uuid.UUID,
	) error {
		if token != *task.LeaseToken {
			t.Fatal("markDone received incorrect lease token")
		}

		return expectedErr
	}

	processTask(context.Background(), worker, task)

	if len(*failed) != 0 {
		t.Fatal(
			"task should not be marked failed when completion update fails",
		)
	}
}

func TestProcessTask_MissingLeaseToken(t *testing.T) {
	task := newTestCleanupTask(4)
	task.LeaseToken = nil

	worker, completed, failed, _ := newTestWorker(t, nil, nil)

	deleteCalled := false

	worker.deleteS3 = func(
		ctx context.Context,
		bucket string,
		key string,
	) error {
		deleteCalled = true
		return nil
	}

	processTask(context.Background(), worker, task)

	if deleteCalled {
		t.Fatal("S3 deletion should not run without a lease token")
	}

	if len(*completed) != 0 || len(*failed) != 0 {
		t.Fatal("task should not be updated without a lease token")
	}
}

func TestProcessAvailableTasks_MultipleTasks(t *testing.T) {
	tasks := []*models.S3CleanupTask{
		newTestCleanupTask(10),
		newTestCleanupTask(11),
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

	worker.claimTask = func(
		ctx context.Context,
	) (*models.S3CleanupTask, error) {
		return nil, expectedErr
	}

	processed := processAvailableTasks(context.Background(), worker)

	if processed {
		t.Fatal("expected no tasks to be processed")
	}
}

func TestProcessTask_DeleteRespectsContext(t *testing.T) {
	task := newTestCleanupTask(20)

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
