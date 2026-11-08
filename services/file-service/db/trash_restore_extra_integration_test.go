package db

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// ------------------------------------------------------------
// TEST 5: Concurrent Restore Folder vs Restore Folder
// ------------------------------------------------------------

func TestIntegration_ConcurrentRestoreSameFolder(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, folderPublicID := env.createFolder(t, nil, "same-folder")

	if err := SoftDeleteFolder(ctx, folderPublicID, env.userID); err != nil {
		t.Fatalf("failed to trash folder: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup

	var successes atomic.Int64
	var failures atomic.Int64

	for i := 0; i < 2; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			err := RestoreFolder(ctx, folderPublicID, env.userID)
			if err == nil {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if successes.Load() != 1 {
		t.Fatalf("expected 1 successful restore, got %d", successes.Load())
	}

	if failures.Load() != 1 {
		t.Fatalf("expected 1 failed restore, got %d", failures.Load())
	}

	env.assertEntryDeleted(t, folderPublicID, false)
	env.assertEventCount(t, "FOLDER_RESTORED", 1)
}

// ------------------------------------------------------------
// TEST 6: Restore Child Before Parent
// ------------------------------------------------------------

func TestIntegration_RestoreChildBeforeParent(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	parentID, parentPublicID := env.createFolder(t, nil, "parent")
	_, childPublicID := env.createFolder(t, &parentID, "child")

	if err := SoftDeleteFolder(ctx, parentPublicID, env.userID); err != nil {
		t.Fatalf("failed to trash parent: %v", err)
	}

	// Child is implicitly trashed with its parent.
	env.assertEntryDeleted(t, childPublicID, true)

	err := RestoreFolder(ctx, childPublicID, env.userID)
	if err == nil {
		t.Fatal("expected restoring child before parent to fail")
	}

	// Neither folder should have changed.
	env.assertEntryDeleted(t, parentPublicID, true)
	env.assertEntryDeleted(t, childPublicID, true)

	env.assertEventCount(t, "FOLDER_RESTORED", 0)
}

// ------------------------------------------------------------
// TEST 7: Concurrent Restore Child vs Trash Parent
// ------------------------------------------------------------

func TestIntegration_ConcurrentRestoreFolderVsTrashParent(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	parentID, parentPublicID := env.createFolder(t, nil, "parent")
	_, childPublicID := env.createFolder(t, &parentID, "child")

	// Independently trash the child first.
	if err := SoftDeleteFolder(ctx, childPublicID, env.userID); err != nil {
		t.Fatalf("failed to trash child: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup

	var restoreErr error
	var trashErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start

		restoreErr = RestoreFolder(ctx, childPublicID, env.userID)
	}()

	go func() {
		defer wg.Done()
		<-start

		trashErr = SoftDeleteFolder(ctx, parentPublicID, env.userID)
	}()

	close(start)
	wg.Wait()

	if trashErr != nil {
		t.Fatalf("parent trash failed: %v", trashErr)
	}

	// Parent must always end up trashed.
	env.assertEntryDeleted(t, parentPublicID, true)

	// Child must also end up trashed:
	// - If restore wins first, parent trash subsequently trashes it.
	// - If parent trash wins first, child restore must fail.
	env.assertEntryDeleted(t, childPublicID, true)

	if restoreErr == nil {
		t.Log("Restore won first; parent trash subsequently trashed child")
	} else {
		t.Logf("Parent trash won first; child restore rejected: %v", restoreErr)
	}

	env.assertEventCount(t, "FOLDER_TRASHED", 2)

	// Restore event is allowed only if restore won the race.
	var restoreEvents int
	err := DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_type = 'ENTRY'
		  AND event_type = 'FOLDER_RESTORED'
		  AND aggregate_id = $1
	`, childPublicID).Scan(&restoreEvents)

	if err != nil {
		t.Fatal(err)
	}

	if restoreEvents < 0 || restoreEvents > 1 {
		t.Fatalf("unexpected child restore event count: %d", restoreEvents)
	}
}

// ------------------------------------------------------------
// TEST 8: Cross-user Restore Must Fail
// ------------------------------------------------------------

func TestIntegration_CrossUserCannotRestoreFile(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, filePublicID := env.createFile(t, nil, "private-file", 100)

	if err := SoftDeleteFile(ctx, filePublicID, env.userID); err != nil {
		t.Fatalf("failed to trash file: %v", err)
	}

	// A different user must not be able to restore this file.
	otherUserID := env.userID + 1

	err := RestoreFile(ctx, filePublicID, otherUserID)
	if err == nil {
		t.Fatal("expected cross-user restore to fail")
	}

	env.assertEntryDeleted(t, filePublicID, true)
	env.assertEventCount(t, "FILE_RESTORED", 0)
}

// ------------------------------------------------------------
// TEST 9: Cross-user Restore Folder Must Fail
// ------------------------------------------------------------

func TestIntegration_CrossUserCannotRestoreFolder(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, folderPublicID := env.createFolder(t, nil, "private-folder")

	if err := SoftDeleteFolder(ctx, folderPublicID, env.userID); err != nil {
		t.Fatalf("failed to trash folder: %v", err)
	}

	otherUserID := env.userID + 1

	err := RestoreFolder(ctx, folderPublicID, otherUserID)
	if err == nil {
		t.Fatal("expected cross-user folder restore to fail")
	}

	env.assertEntryDeleted(t, folderPublicID, true)
	env.assertEventCount(t, "FOLDER_RESTORED", 0)
}
