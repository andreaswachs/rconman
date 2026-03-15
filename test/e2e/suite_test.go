package e2e

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// TODO: Setup test environment
	// - Connect to rconman API
	// - Initialize test fixtures
	code := m.Run()
	// TODO: Cleanup

	os.Exit(code)
}
