package lifecycle

import (
	"fmt"
	"time"
)

func StartExpirationScheduler(store *SQLiteStore) {
	go func() {
		for {
			time.Sleep(5 * time.Second)

			leases, err := store.GetExpiredLeases()
			if err != nil {
				continue
			}

			for _, lease := range leases {
				fmt.Println(">>> Auto deleting expired lease:", lease.LeaseID)
				store.MarkDeleted(lease.LeaseID)
			}
		}
	}()
}

