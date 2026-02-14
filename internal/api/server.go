package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lvbu1984/szem-core/internal/lifecycle"
	"github.com/lvbu1984/szem-core/internal/storage"
)

const maxUploadSize = 50 << 20 // 50MB

type Server struct {
	store   *lifecycle.SQLiteStore
	adapter storage.Adapter
}

func NewServer(store *lifecycle.SQLiteStore, adapter storage.Adapter) *Server {
	return &Server{
		store:   store,
		adapter: adapter,
	}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.withMiddleware(s.handleHealth))
	mux.HandleFunc("/upload", s.withMiddleware(s.handleUpload))
	mux.HandleFunc("/object/", s.withMiddleware(s.handleGetObject))
	mux.HandleFunc("/objects", s.withMiddleware(s.handleListObjects))
	mux.HandleFunc("/dashboard", s.withMiddleware(s.handleDashboard))

	log.Println("Qave API running on", addr)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) withMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		start := time.Now()

		w.Header().Set("X-Request-Id", requestID)

		next(w, r)

		log.Printf(
			"request_id=%s method=%s path=%s latency_ms=%d",
			requestID,
			r.Method,
			r.URL.Path,
			time.Since(start).Milliseconds(),
		)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	wallet := r.Header.Get("X-Wallet")
	if wallet == "" {
		writeError(w, http.StatusBadRequest, "missing_wallet", "X-Wallet header required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "max file size is 50MB")
		return
	}

	ctx := context.Background()

	meta := storage.DataSetMeta{
		Application: "Qave",
		Version:     "1.0",
		WithCDN:     false,
	}

	dataSetID, err := s.adapter.EnsureDataSet(ctx, meta)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dataset_error", "failed to ensure dataset")
		return
	}

	uploadResult, err := s.adapter.Upload(ctx, dataSetID, data, storage.UploadOptions{
		FileName: "file",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_error", "upload failed")
		return
	}

	objectID := uuid.New().String()

	s.store.InsertUser(wallet)
	s.store.InsertDataSet(string(dataSetID), wallet)
	s.store.InsertObject(objectID, wallet, string(dataSetID), int64(len(data)))

	now := time.Now().UTC()
	expire := now.Add(30 * 24 * time.Hour)

	lease := lifecycle.ObjectLease{
		LeaseID:  uuid.New().String(),
		ObjectID: objectID,
		Wallet:   wallet,
		CreatedAt: now,
		ExpireAt:  expire,
		StorageRef: lifecycle.StorageRef{
			DataSetID: string(dataSetID),
			PieceCID:  string(uploadResult.PieceCID),
		},
	}

	s.store.InsertLease(lease)

	writeJSON(w, map[string]any{
		"object_id": objectID,
		"piece_cid": uploadResult.PieceCID,
		"size":      uploadResult.Size,
		"expire_at": expire,
	})
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request) {
	objectID := strings.TrimPrefix(r.URL.Path, "/object/")

	lease, err := s.store.GetActiveLeaseByObjectID(objectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	status := lifecycle.CalculateLeaseStatus(*lease)
	if status != lifecycle.LeaseActive {
		writeError(w, http.StatusNotFound, "not_found", "object not found")
		return
	}

	data, err := s.adapter.Download(context.Background(), storage.PieceCID(lease.StorageRef.PieceCID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "download_error", "download failed")
		return
	}

	w.Write(data)
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	wallet := r.Header.Get("X-Wallet")
	if wallet == "" {
		writeError(w, http.StatusBadRequest, "missing_wallet", "X-Wallet header required")
		return
	}

	rows, err := s.store.DB().Query(`
SELECT o.object_id, o.size_bytes, l.created_at, l.expire_at, l.deleted_at
FROM objects o
JOIN leases l ON o.object_id = l.object_id
WHERE o.wallet = ?
ORDER BY l.created_at DESC
`, wallet)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "failed to query objects")
		return
	}
	defer rows.Close()

	var result []map[string]any

	for rows.Next() {
		var objectID string
		var size int64
		var createdStr, expireStr string
		var deletedStr *string

		rows.Scan(&objectID, &size, &createdStr, &expireStr, &deletedStr)

		createdAt, _ := time.Parse(time.RFC3339Nano, createdStr)
		expireAt, _ := time.Parse(time.RFC3339Nano, expireStr)

		var deletedAt *time.Time
		if deletedStr != nil {
			t, _ := time.Parse(time.RFC3339Nano, *deletedStr)
			deletedAt = &t
		}

		lease := lifecycle.ObjectLease{
			ObjectID: objectID,
			CreatedAt: createdAt,
			ExpireAt:  expireAt,
			DeletedAt: deletedAt,
		}

		status := lifecycle.CalculateLeaseStatus(lease)

		result = append(result, map[string]any{
			"object_id": objectID,
			"size":      size,
			"created_at": createdAt,
			"expire_at":  expireAt,
			"status":     status,
		})
	}

	writeJSON(w, result)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetExtendedStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats_error", "failed to get stats")
		return
	}
	writeJSON(w, stats)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, errCode, message string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]string{
		"error":   errCode,
		"message": message,
	})
}

