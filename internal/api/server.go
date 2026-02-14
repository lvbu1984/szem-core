package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lvbu1984/szem-core/internal/lifecycle"
	"github.com/lvbu1984/szem-core/internal/storage"
)

type Server struct {
	Store   *lifecycle.SQLiteStore
	Adapter storage.Adapter
}

func NewServer(store *lifecycle.SQLiteStore, adapter storage.Adapter) *Server {
	return &Server{
		Store:   store,
		Adapter: adapter,
	}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/upload", s.handleUpload)
	http.HandleFunc("/dashboard", s.handleDashboard)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	wallet := r.Header.Get("X-Wallet")
	if wallet == "" {
		http.Error(w, "missing X-Wallet header", 400)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", 500)
		return
	}

	ctx := r.Context()

	meta := storage.DataSetMeta{
		Application: "Qave",
		Version:     "1.0",
		WithCDN:     false,
	}

	dataSetID, err := s.Adapter.EnsureDataSet(ctx, meta)
	if err != nil {
		http.Error(w, "dataset error", 500)
		return
	}

	s.Store.InsertUser(wallet)
	s.Store.InsertDataSet(string(dataSetID), wallet)

	uploadResult, err := s.Adapter.Upload(ctx, dataSetID, body, storage.UploadOptions{
		FileName: "file.bin",
	})
	if err != nil {
		http.Error(w, "upload error", 500)
		return
	}

	objectID := uuid.New().String()

	s.Store.InsertObject(
		objectID,
		wallet,
		string(dataSetID),
		int64(uploadResult.Size),
	)

	lease := lifecycle.ObjectLease{
		LeaseID:  uuid.New().String(),
		ObjectID: objectID,
		Bucket:   "qave",
		Key:      string(uploadResult.PieceCID),
		Wallet:   wallet,
		CreatedAt: time.Now().UTC(),
		ExpireAt:  time.Now().UTC().Add(30 * 24 * time.Hour),
		StorageRef: lifecycle.StorageRef{
			DataSetID: string(dataSetID),
			PieceCID:  string(uploadResult.PieceCID),
		},
	}

	s.Store.InsertLease(lease)

	resp := map[string]any{
		"object_id": objectID,
		"piece_cid": uploadResult.PieceCID,
		"size":      uploadResult.Size,
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, _ := s.Store.GetInternalStats()
	json.NewEncoder(w).Encode(stats)
}

