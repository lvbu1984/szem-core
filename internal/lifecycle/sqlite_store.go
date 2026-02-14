package lifecycle

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS users (
	wallet TEXT PRIMARY KEY,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS datasets (
	dataset_id TEXT PRIMARY KEY,
	wallet TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS objects (
	object_id TEXT PRIMARY KEY,
	wallet TEXT NOT NULL,
	dataset_id TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS leases (
	lease_id TEXT PRIMARY KEY,
	object_id TEXT NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	wallet TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expire_at TEXT NOT NULL,
	dataset_id TEXT,
	piece_cid TEXT
);
`)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func iso(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// =========================
// Insert Methods
// =========================

func (s *SQLiteStore) InsertUser(wallet string) {
	_, _ = s.db.Exec(
		`INSERT OR IGNORE INTO users(wallet, created_at) VALUES (?, ?)`,
		wallet,
		iso(time.Now()),
	)
}

func (s *SQLiteStore) InsertDataSet(datasetID, wallet string) {
	_, _ = s.db.Exec(
		`INSERT OR IGNORE INTO datasets(dataset_id, wallet, created_at) VALUES (?, ?, ?)`,
		datasetID,
		wallet,
		iso(time.Now()),
	)
}

func (s *SQLiteStore) InsertObject(objectID, wallet, datasetID string, size int64) {
	_, _ = s.db.Exec(
		`INSERT INTO objects(object_id, wallet, dataset_id, size_bytes, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		objectID,
		wallet,
		datasetID,
		size,
		iso(time.Now()),
	)
}

func (s *SQLiteStore) InsertLease(l ObjectLease) {
	_, _ = s.db.Exec(`
INSERT INTO leases(
	lease_id, object_id, bucket, object_key, wallet,
	created_at, expire_at, dataset_id, piece_cid
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		l.LeaseID,
		l.ObjectID,
		l.Bucket,
		l.Key,
		l.Wallet,
		iso(l.CreatedAt),
		iso(l.ExpireAt),
		l.StorageRef.DataSetID,
		l.StorageRef.PieceCID,
	)
}

// =========================
// Dashboard
// =========================

type InternalStats struct {
	TotalUsers        int64
	TotalStorageBytes int64
	NewUsersToday     int64
	StorageTodayBytes int64
	ExpiringIn7Days   int64
}

func (s *SQLiteStore) GetInternalStats() (*InternalStats, error) {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	in7days := now.Add(7 * 24 * time.Hour)

	stats := &InternalStats{}

	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers)

	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE created_at >= ?`,
		iso(today),
	).Scan(&stats.NewUsersToday)

	_ = s.db.QueryRow(
		`SELECT COALESCE(SUM(size_bytes),0) FROM objects`,
	).Scan(&stats.TotalStorageBytes)

	_ = s.db.QueryRow(
		`SELECT COALESCE(SUM(size_bytes),0) FROM objects WHERE created_at >= ?`,
		iso(today),
	).Scan(&stats.StorageTodayBytes)

	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM leases WHERE expire_at BETWEEN ? AND ?`,
		iso(now),
		iso(in7days),
	).Scan(&stats.ExpiringIn7Days)

	return stats, nil
}

// =========================
// Expiration + Deletion
// =========================

func (s *SQLiteStore) GetExpiredLeases() ([]ObjectLease, error) {
	rows, err := s.db.Query(`
SELECT lease_id, object_id, bucket, object_key, wallet,
       created_at, expire_at, dataset_id, piece_cid
FROM leases
WHERE expire_at <= ? AND deleted_at IS NULL
`, iso(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leases []ObjectLease

	for rows.Next() {
		var l ObjectLease
		var createdStr, expireStr string
		var datasetID, pieceCID string

		if err := rows.Scan(
			&l.LeaseID,
			&l.ObjectID,
			&l.Bucket,
			&l.Key,
			&l.Wallet,
			&createdStr,
			&expireStr,
			&datasetID,
			&pieceCID,
		); err != nil {
			continue
		}

		createdAt, _ := time.Parse(time.RFC3339Nano, createdStr)
		expireAt, _ := time.Parse(time.RFC3339Nano, expireStr)

		l.CreatedAt = createdAt
		l.ExpireAt = expireAt
		l.StorageRef = StorageRef{
			DataSetID: datasetID,
			PieceCID:  pieceCID,
		}

		leases = append(leases, l)
	}

	return leases, nil
}

func (s *SQLiteStore) MarkDeleted(leaseID string) {
	_, _ = s.db.Exec(
		`UPDATE leases SET deleted_at = ? WHERE lease_id = ?`,
		iso(time.Now()),
		leaseID,
	)
}

