package store

import (
	"context"
	"testing"
	"time"
)

// MockStore is an in-memory implementation of Store for testing.
type MockStore struct {
	logs []CommandLog
}

// NewMockStore creates a new MockStore instance.
func NewMockStore() *MockStore {
	return &MockStore{
		logs: make([]CommandLog, 0),
	}
}

// RecordCommand adds a command log to the mock store.
func (m *MockStore) RecordCommand(ctx context.Context, email, serverID, command, response string, durationMS int64) error {
	log := CommandLog{
		ID:        int64(len(m.logs) + 1),
		Timestamp: time.Now(),
		UserEmail: email,
		ServerID:  serverID,
		Command:   command,
		Response:  response,
		DurationMS: durationMS,
	}
	m.logs = append(m.logs, log)
	return nil
}

// GetLogs retrieves the most recent command logs up to the limit.
func (m *MockStore) GetLogs(ctx context.Context, limit int) ([]CommandLog, error) {
	if limit > len(m.logs) {
		return m.logs, nil
	}
	return m.logs[len(m.logs)-limit:], nil
}

// PruneOlderThan removes logs older than the specified duration.
func (m *MockStore) PruneOlderThan(ctx context.Context, age time.Duration) error {
	cutoff := time.Now().Add(-age)
	filtered := make([]CommandLog, 0)
	for _, log := range m.logs {
		if log.Timestamp.After(cutoff) {
			filtered = append(filtered, log)
		}
	}
	m.logs = filtered
	return nil
}

// TestMockStoreRecordCommand tests recording a command.
func TestMockStoreRecordCommand(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	err := store.RecordCommand(ctx, "user@example.com", "server1", "list", "response", 100)
	if err != nil {
		t.Fatalf("RecordCommand failed: %v", err)
	}

	logs, err := store.GetLogs(ctx, 10)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}

	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	if logs[0].UserEmail != "user@example.com" {
		t.Fatalf("expected user_email 'user@example.com', got '%s'", logs[0].UserEmail)
	}
}

// TestMockStoreGetLogs tests retrieving logs with a limit.
func TestMockStoreGetLogs(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		store.RecordCommand(ctx, "user@example.com", "server1", "cmd", "resp", int64(i*10))
	}

	logs, err := store.GetLogs(ctx, 2)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}

	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
}

// TestMockStorePruneOlderThan tests deleting old logs.
func TestMockStorePruneOlderThan(t *testing.T) {
	store := NewMockStore()
	ctx := context.Background()

	// Add a log with an old timestamp
	oldLog := CommandLog{
		ID:        1,
		Timestamp: time.Now().Add(-2 * time.Hour),
		UserEmail: "old@example.com",
		ServerID:  "server1",
		Command:   "cmd",
		Response:  "resp",
		DurationMS: 100,
	}
	store.logs = append(store.logs, oldLog)

	// Add a recent log
	store.RecordCommand(ctx, "new@example.com", "server1", "cmd", "resp", 100)

	// Prune logs older than 1 hour
	err := store.PruneOlderThan(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("PruneOlderThan failed: %v", err)
	}

	logs, _ := store.GetLogs(ctx, 10)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log after prune, got %d", len(logs))
	}

	if logs[0].UserEmail != "new@example.com" {
		t.Fatalf("expected new@example.com, got %s", logs[0].UserEmail)
	}
}
