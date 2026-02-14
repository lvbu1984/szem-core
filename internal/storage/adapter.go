package storage

import "context"

type DataSetID string
type PieceCID string

type UploadOptions struct {
	FileName string
}

type UploadResult struct {
	PieceCID PieceCID
	Size     int
}

type DataSetMeta struct {
	Application string
	Version     string
	WithCDN     bool
}

type Adapter interface {
	EnsureDataSet(ctx context.Context, meta DataSetMeta) (DataSetID, error)
	Upload(ctx context.Context, dataSetID DataSetID, data []byte, opts UploadOptions) (*UploadResult, error)
	Download(ctx context.Context, pieceCID PieceCID) ([]byte, error)
	Delete(ctx context.Context, pieceCID PieceCID) error
}

