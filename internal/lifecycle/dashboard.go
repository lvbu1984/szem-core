package lifecycle

import "time"

type DashboardStats struct {
	TotalUsers        int64
	NewUsersToday     int64
	TotalStorageBytes int64
	StorageTodayBytes int64
	ExpiringIn7Days   int64
}

func (s *SQLiteStore) GetDashboardStats() (*DashboardStats, error) {
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	in7days := now.Add(7 * 24 * time.Hour)

	stats := &DashboardStats{}

	// total users
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers)

	// new users today
	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM users WHERE created_at >= ?`,
		iso(today),
	).Scan(&stats.NewUsersToday)

	// total storage
	_ = s.db.QueryRow(
		`SELECT COALESCE(SUM(size_bytes),0) FROM objects`,
	).Scan(&stats.TotalStorageBytes)

	// today storage
	_ = s.db.QueryRow(
		`SELECT COALESCE(SUM(size_bytes),0) FROM objects WHERE created_at >= ?`,
		iso(today),
	).Scan(&stats.StorageTodayBytes)

	// expiring in 7 days
	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM leases WHERE expire_at BETWEEN ? AND ? AND deleted_at IS NULL`,
		iso(now),
		iso(in7days),
	).Scan(&stats.ExpiringIn7Days)

	return stats, nil
}

