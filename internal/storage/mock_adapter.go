package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type MockAdapter struct {
	data map[PieceCID][]byte
	mu   sync.Mutex
}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		data: make(map[PieceCID][]byte),
	}
}

func (m *MockAdapter) EnsureDataSet(ctx context.Context, meta DataSetMeta) (DataSetID, error) {
	return DataSetID("mock-ds-1"), nil
}

func (m *MockAdapter) Upload(ctx context.Context, dataSetID DataSetID, data []byte, opts UploadOptions) (*UploadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pieceCID := PieceCID(fmt.Sprintf("mock-piece-%d", len(m.data)+1))
	m.data[pieceCID] = data

	return &UploadResult{
		PieceCID: pieceCID,
		Size:     len(data),
	}, nil
}

func (m *MockAdapter) Download(ctx context.Context, pieceCID PieceCID) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.data[pieceCID]
	if !ok {
		return nil, errors.New("not found")
	}
	return data, nil
}

func (m *MockAdapter) Delete(ctx context.Context, pieceCID PieceCID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, pieceCID)
	return nil
}

