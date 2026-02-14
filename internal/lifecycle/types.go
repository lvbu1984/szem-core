package lifecycle

import "time"

// LeaseStatus is the explicit, auditable state of a lease.
// We intentionally keep it small for MVP; future states can be added later.
type LeaseStatus string

const (
	LeaseActive  LeaseStatus = "active"
	LeaseExpired LeaseStatus = "expired"
	LeaseDeleted LeaseStatus = "deleted"
)

// CalculateLeaseStatus is the single source of truth.
// All API handlers / workers must use this function to interpret a lease.
func CalculateLeaseStatus(l ObjectLease) LeaseStatus {
	now := time.Now().UTC()

	if l.DeletedAt != nil {
		return LeaseDeleted
	}

	// no grace period: once expired, it is not readable.
	if !l.ExpireAt.IsZero() && (now.Equal(l.ExpireAt) || now.After(l.ExpireAt)) {
		return LeaseExpired
	}

	return LeaseActive
}

// ObjectLease is the core lifecycle record binding an object to storage identity.
type ObjectLease struct {
	LeaseID  string
	ObjectID string

	// For future S3-compat mapping; Qave may not use them directly today,
	// but keeping them here keeps the model explicit/auditable.
	Bucket string
	Key    string

	Wallet string

	CreatedAt time.Time
	ExpireAt  time.Time

	// TombstonedAt means: object must be immediately invisible for GET/LIST.
	// (We may not persist this yet, but keeping field here keeps model complete.)
	TombstonedAt *time.Time

	// DeletedAt means: physical deletion confirmed (or mock-confirmed).
	DeletedAt *time.Time

	// StorageRef binds this lease/object to FWSS piece identity.
	// Keep explicit so the system is not a black box.
	StorageRef StorageRef
}

type StorageRef struct {
	DataSetID string
	PieceCID  string
}

