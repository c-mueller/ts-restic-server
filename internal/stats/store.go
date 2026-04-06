package stats

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// RepoStats holds cumulative traffic metrics for a single repository.
type RepoStats struct {
	RepoPath     string    `json:"repo_path"`
	BytesWritten int64     `json:"bytes_written"`
	BytesRead    int64     `json:"bytes_read"`
	BytesDeleted int64     `json:"bytes_deleted"`
	WriteCount   int64     `json:"write_count"`
	ReadCount    int64     `json:"read_count"`
	DeleteCount  int64     `json:"delete_count"`
	LastAccess   time.Time `json:"last_access"`
	CreatedAt    time.Time `json:"created_at"`
}

// Store is a SQLite-backed stats store that records per-repository
// traffic metrics. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dbPath and initializes
// the stats schema. It enables WAL mode for better concurrent access.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open stats db: %w", err)
	}

	// WAL mode allows concurrent reads during writes.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Wait up to 5 seconds for locks rather than failing immediately.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Serialize writes through a single connection to avoid SQLITE_BUSY
	// under concurrent access. Reads still benefit from WAL mode.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate stats schema: %w", err)
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS repo_stats (
			repo_path     TEXT PRIMARY KEY,
			bytes_written INTEGER NOT NULL DEFAULT 0,
			bytes_read    INTEGER NOT NULL DEFAULT 0,
			bytes_deleted INTEGER NOT NULL DEFAULT 0,
			write_count   INTEGER NOT NULL DEFAULT 0,
			read_count    INTEGER NOT NULL DEFAULT 0,
			delete_count  INTEGER NOT NULL DEFAULT 0,
			last_access   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// RecordWrite increments write counters for the given repository.
func (s *Store) RecordWrite(repoPath string, bytes int64) error {
	_, err := s.db.Exec(`
		INSERT INTO repo_stats (repo_path, bytes_written, write_count, last_access)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(repo_path) DO UPDATE SET
			bytes_written = bytes_written + excluded.bytes_written,
			write_count = write_count + 1,
			last_access = CURRENT_TIMESTAMP
	`, repoPath, bytes)
	return err
}

// RecordRead increments read counters for the given repository.
func (s *Store) RecordRead(repoPath string, bytes int64) error {
	_, err := s.db.Exec(`
		INSERT INTO repo_stats (repo_path, bytes_read, read_count, last_access)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(repo_path) DO UPDATE SET
			bytes_read = bytes_read + excluded.bytes_read,
			read_count = read_count + 1,
			last_access = CURRENT_TIMESTAMP
	`, repoPath, bytes)
	return err
}

// RecordDelete increments delete counters for the given repository.
func (s *Store) RecordDelete(repoPath string, bytes int64) error {
	_, err := s.db.Exec(`
		INSERT INTO repo_stats (repo_path, bytes_deleted, delete_count, last_access)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(repo_path) DO UPDATE SET
			bytes_deleted = bytes_deleted + excluded.bytes_deleted,
			delete_count = delete_count + 1,
			last_access = CURRENT_TIMESTAMP
	`, repoPath, bytes)
	return err
}

// GetRepoStats returns stats for a single repository.
// Returns nil, nil if the repository has no recorded stats.
func (s *Store) GetRepoStats(repoPath string) (*RepoStats, error) {
	row := s.db.QueryRow(`
		SELECT repo_path, bytes_written, bytes_read, bytes_deleted,
		       write_count, read_count, delete_count, last_access, created_at
		FROM repo_stats WHERE repo_path = ?
	`, repoPath)

	var rs RepoStats
	err := row.Scan(&rs.RepoPath, &rs.BytesWritten, &rs.BytesRead, &rs.BytesDeleted,
		&rs.WriteCount, &rs.ReadCount, &rs.DeleteCount, &rs.LastAccess, &rs.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rs, nil
}

// GetAllRepoStats returns stats for all repositories ordered by last access.
func (s *Store) GetAllRepoStats() ([]RepoStats, error) {
	rows, err := s.db.Query(`
		SELECT repo_path, bytes_written, bytes_read, bytes_deleted,
		       write_count, read_count, delete_count, last_access, created_at
		FROM repo_stats ORDER BY last_access DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RepoStats
	for rows.Next() {
		var rs RepoStats
		if err := rows.Scan(&rs.RepoPath, &rs.BytesWritten, &rs.BytesRead, &rs.BytesDeleted,
			&rs.WriteCount, &rs.ReadCount, &rs.DeleteCount, &rs.LastAccess, &rs.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, rs)
	}
	return result, rows.Err()
}

// GetSummary returns aggregate stats across all repositories.
func (s *Store) GetSummary() (*RepoStats, error) {
	row := s.db.QueryRow(`
		SELECT COALESCE(SUM(bytes_written), 0), COALESCE(SUM(bytes_read), 0),
		       COALESCE(SUM(bytes_deleted), 0), COALESCE(SUM(write_count), 0),
		       COALESCE(SUM(read_count), 0), COALESCE(SUM(delete_count), 0),
		       COUNT(*)
		FROM repo_stats
	`)

	var rs RepoStats
	var repoCount int64
	err := row.Scan(&rs.BytesWritten, &rs.BytesRead, &rs.BytesDeleted,
		&rs.WriteCount, &rs.ReadCount, &rs.DeleteCount, &repoCount)
	if err != nil {
		return nil, err
	}
	rs.RepoPath = fmt.Sprintf("(%d repositories)", repoCount)
	return &rs, nil
}
