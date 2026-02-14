package lifecycle

import "time"

type StorageRef struct {
	DataSetID string
	PieceCID  string
}

type ObjectLease struct {
	LeaseID  string
	ObjectID string
	Bucket   string
	Key      string
	Wallet   string

	CreatedAt time.Time
	ExpireAt  time.Time

	TombstonedAt *time.Time
	DeletedAt    *time.Time

	StorageRef StorageRef
}

