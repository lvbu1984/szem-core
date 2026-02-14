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

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
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
	bucket TEXT,
	object_key TEXT,
	wallet TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expire_at TEXT NOT NULL,
	deleted_at TEXT,
	dataset_id TEXT,
	piece_cid TEXT
);
`)
	return err
}

func iso(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

//
// ============================
// INSERT METHODS
// ============================
//

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
	created_at, expire_at, deleted_at, dataset_id, piece_cid
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		l.LeaseID,
		l.ObjectID,
		l.Bucket,
		l.Key,
		l.Wallet,
		iso(l.CreatedAt),
		iso(l.ExpireAt),
		nil,
		l.StorageRef.DataSetID,
		l.StorageRef.PieceCID,
	)
}

//
// ============================
// LEASE QUERIES
// ============================
//

func (s *SQLiteStore) GetActiveLeaseByObjectID(objectID string) (*ObjectLease, error) {
	row := s.db.QueryRow(`
SELECT lease_id, object_id, wallet, created_at, expire_at, deleted_at, dataset_id, piece_cid
FROM leases
WHERE object_id = ?
ORDER BY created_at DESC
LIMIT 1
`, objectID)

	var lease ObjectLease
	var createdStr, expireStr string
	var deletedStr sql.NullString
	var datasetID, pieceCID string

	err := row.Scan(
		&lease.LeaseID,
		&lease.ObjectID,
		&lease.Wallet,
		&createdStr,
		&expireStr,
		&deletedStr,
		&datasetID,
		&pieceCID,
	)
	if err != nil {
		return nil, err
	}

	lease.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	lease.ExpireAt, _ = time.Parse(time.RFC3339Nano, expireStr)

	if deletedStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, deletedStr.String)
		lease.DeletedAt = &t
	}

	lease.StorageRef = StorageRef{
		DataSetID: datasetID,
		PieceCID:  pieceCID,
	}

	return &lease, nil
}

//
// ============================
// DASHBOARD (EXTENDED STATS)
// ============================
//

type ExtendedStats struct {
	TotalUsers        int64
	TotalStorageBytes int64
	NewUsersToday     int64
	StorageTodayBytes int64
	ExpiringIn7Days   int64

	ActiveObjects  int64
	ExpiredObjects int64
	DeletedObjects int64
}

func (s *SQLiteStore) GetExtendedStats() (*ExtendedStats, error) {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	in7days := now.Add(7 * 24 * time.Hour)

	stats := &ExtendedStats{}

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

	rows, err := s.db.Query(`
SELECT lease_id, object_id, wallet, created_at, expire_at, deleted_at
FROM leases
`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var lease ObjectLease
		var createdStr, expireStr string
		var deletedStr sql.NullString

		rows.Scan(
			&lease.LeaseID,
			&lease.ObjectID,
			&lease.Wallet,
			&createdStr,
			&expireStr,
			&deletedStr,
		)

		lease.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		lease.ExpireAt, _ = time.Parse(time.RFC3339Nano, expireStr)

		if deletedStr.Valid {
			t, _ := time.Parse(time.RFC3339Nano, deletedStr.String)
			lease.DeletedAt = &t
		}

		status := CalculateLeaseStatus(lease)

		switch status {
		case LeaseActive:
			stats.ActiveObjects++
		case LeaseExpired:
			stats.ExpiredObjects++
		case LeaseDeleted:
			stats.DeletedObjects++
		}
	}

	return stats, nil
}

