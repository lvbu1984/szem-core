package storage

import (
	"context"
	"fmt"
	"time"
)

type MockAdapter struct{}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{}
}

func (m *MockAdapter) EnsureDataSet(ctx context.Context, meta DataSetMeta) (DataSetID, error) {
	return DataSetID("mock-ds-1"), nil
}

func (m *MockAdapter) Upload(ctx context.Context, dataSetID DataSetID, data []byte, opts UploadOptions) (*UploadResult, error) {
	return &UploadResult{
		PieceCID: PieceCID(fmt.Sprintf("mock-piece-%d", time.Now().UnixNano())),
		Size:     len(data),
	}, nil
}

