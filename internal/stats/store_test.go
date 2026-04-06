package stats

import (
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-stats.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordWrite(t *testing.T) {
	s := newTestStore(t)

	if err := s.RecordWrite("/repo-a", 1024); err != nil {
		t.Fatalf("RecordWrite: %v", err)
	}
	if err := s.RecordWrite("/repo-a", 2048); err != nil {
		t.Fatalf("RecordWrite: %v", err)
	}

	rs, err := s.GetRepoStats("/repo-a")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if rs == nil {
		t.Fatal("expected stats, got nil")
	}
	if rs.BytesWritten != 3072 {
		t.Errorf("BytesWritten = %d, want 3072", rs.BytesWritten)
	}
	if rs.WriteCount != 2 {
		t.Errorf("WriteCount = %d, want 2", rs.WriteCount)
	}
	if rs.BytesRead != 0 {
		t.Errorf("BytesRead = %d, want 0", rs.BytesRead)
	}
}

func TestRecordRead(t *testing.T) {
	s := newTestStore(t)

	if err := s.RecordRead("/repo-b", 512); err != nil {
		t.Fatalf("RecordRead: %v", err)
	}

	rs, err := s.GetRepoStats("/repo-b")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if rs.BytesRead != 512 {
		t.Errorf("BytesRead = %d, want 512", rs.BytesRead)
	}
	if rs.ReadCount != 1 {
		t.Errorf("ReadCount = %d, want 1", rs.ReadCount)
	}
}

func TestRecordDelete(t *testing.T) {
	s := newTestStore(t)

	if err := s.RecordDelete("/repo-c", 4096); err != nil {
		t.Fatalf("RecordDelete: %v", err)
	}
	if err := s.RecordDelete("/repo-c", 2048); err != nil {
		t.Fatalf("RecordDelete: %v", err)
	}

	rs, err := s.GetRepoStats("/repo-c")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if rs.BytesDeleted != 6144 {
		t.Errorf("BytesDeleted = %d, want 6144", rs.BytesDeleted)
	}
	if rs.DeleteCount != 2 {
		t.Errorf("DeleteCount = %d, want 2", rs.DeleteCount)
	}
}

func TestGetRepoStats_NotFound(t *testing.T) {
	s := newTestStore(t)

	rs, err := s.GetRepoStats("/nonexistent")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}
	if rs != nil {
		t.Errorf("expected nil for nonexistent repo, got %+v", rs)
	}
}

func TestGetAllRepoStats(t *testing.T) {
	s := newTestStore(t)

	s.RecordWrite("/repo-1", 100)
	s.RecordWrite("/repo-2", 200)
	s.RecordRead("/repo-3", 300)

	all, err := s.GetAllRepoStats()
	if err != nil {
		t.Fatalf("GetAllRepoStats: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("GetAllRepoStats len = %d, want 3", len(all))
	}
}

func TestGetSummary(t *testing.T) {
	s := newTestStore(t)

	s.RecordWrite("/repo-1", 100)
	s.RecordWrite("/repo-2", 200)
	s.RecordRead("/repo-1", 50)
	s.RecordDelete("/repo-2", 75)

	summary, err := s.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.BytesWritten != 300 {
		t.Errorf("BytesWritten = %d, want 300", summary.BytesWritten)
	}
	if summary.BytesRead != 50 {
		t.Errorf("BytesRead = %d, want 50", summary.BytesRead)
	}
	if summary.BytesDeleted != 75 {
		t.Errorf("BytesDeleted = %d, want 75", summary.BytesDeleted)
	}
	if summary.WriteCount != 2 {
		t.Errorf("WriteCount = %d, want 2", summary.WriteCount)
	}
}

func TestGetSummary_Empty(t *testing.T) {
	s := newTestStore(t)

	summary, err := s.GetSummary()
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.BytesWritten != 0 || summary.BytesRead != 0 || summary.BytesDeleted != 0 {
		t.Errorf("expected zero summary for empty db, got %+v", summary)
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := newTestStore(t)

	const goroutines = 10
	const writesPerGoroutine = 50
	const bytesPerWrite = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				s.RecordWrite("/concurrent-repo", bytesPerWrite)
			}
		}()
	}
	wg.Wait()

	rs, err := s.GetRepoStats("/concurrent-repo")
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}

	wantBytes := int64(goroutines * writesPerGoroutine * bytesPerWrite)
	wantCount := int64(goroutines * writesPerGoroutine)

	if rs.BytesWritten != wantBytes {
		t.Errorf("BytesWritten = %d, want %d", rs.BytesWritten, wantBytes)
	}
	if rs.WriteCount != wantCount {
		t.Errorf("WriteCount = %d, want %d", rs.WriteCount, wantCount)
	}
}

func TestMixedOperations(t *testing.T) {
	s := newTestStore(t)

	repo := "/mixed"
	s.RecordWrite(repo, 1000)
	s.RecordRead(repo, 500)
	s.RecordDelete(repo, 200)
	s.RecordWrite(repo, 300)
	s.RecordRead(repo, 100)

	rs, err := s.GetRepoStats(repo)
	if err != nil {
		t.Fatalf("GetRepoStats: %v", err)
	}

	if rs.BytesWritten != 1300 {
		t.Errorf("BytesWritten = %d, want 1300", rs.BytesWritten)
	}
	if rs.BytesRead != 600 {
		t.Errorf("BytesRead = %d, want 600", rs.BytesRead)
	}
	if rs.BytesDeleted != 200 {
		t.Errorf("BytesDeleted = %d, want 200", rs.BytesDeleted)
	}
	if rs.WriteCount != 2 {
		t.Errorf("WriteCount = %d, want 2", rs.WriteCount)
	}
	if rs.ReadCount != 2 {
		t.Errorf("ReadCount = %d, want 2", rs.ReadCount)
	}
	if rs.DeleteCount != 1 {
		t.Errorf("DeleteCount = %d, want 1", rs.DeleteCount)
	}
}
