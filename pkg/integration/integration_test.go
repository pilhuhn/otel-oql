package integration

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/pinot"
)

// TestMain handles setup and teardown for integration tests
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("⚠️  skip integration tests: -short")
		os.Exit(0)
	}

	// Check if Pinot is running
	if !isPinotAvailable() {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			fmt.Println("❌ Pinot is not running or not accessible at " + pinotBrokerURL)
			fmt.Println("Start Pinot with: docker-compose up -d")
			fmt.Println("Then ensure schemas are created: ./otel-oql setup-schema --pinot-url=" + pinotControllerURL)
			os.Exit(1)
		}
		fmt.Println("⚠️  skip integration tests: Pinot not reachable at " + pinotBrokerURL)
		fmt.Println("Start Pinot with: docker-compose up -d, then: go test ./pkg/integration/... -count=1")
		fmt.Println("Or set REQUIRE_INTEGRATION=1 to fail when Pinot is down (e.g. CI with Pinot).")
		os.Exit(0)
	}

	fmt.Println("✅ Pinot is running and accessible")

	// Verify schemas exist
	if err := verifySchemas(); err != nil {
		fmt.Println("❌ Schema verification failed:", err)
		fmt.Println("Create schemas with: ./otel-oql setup-schema --pinot-url=" + pinotControllerURL)
		os.Exit(1)
	}

	fmt.Println("✅ All required schemas exist")

	// Check if OTEL-OQL service is running
	if !isOtelOQLAvailable() {
		fmt.Println("⚠️  OTEL-OQL service is not running")
		fmt.Println("Start service with: ./otel-oql --test-mode --pinot-url=" + pinotBrokerURL + " --kafka-brokers=localhost:9092")
		fmt.Println("Some tests will be skipped")
	} else {
		fmt.Println("✅ OTEL-OQL service is running")
	}

	// Clean up old test data before running tests
	fmt.Println("🧹 Cleaning up old test data...")
	cleanupTestTenants()

	// Run tests
	code := m.Run()

	// Cleanup after tests (optional - might want to leave data for inspection)
	// cleanupTestTenants()

	os.Exit(code)
}

// isPinotAvailable checks if Pinot is running and accessible
func isPinotAvailable() bool {
	// Just check if Pinot health endpoint responds
	resp, err := http.Get(pinotBrokerURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// isOtelOQLAvailable checks if the OTEL-OQL service is running
func isOtelOQLAvailable() bool {
	// Try to connect to the query API
	_, err := QueryOQL(&testing.T{}, "signal=spans | limit 1", testTenantID)
	return err == nil
}

// verifySchemas checks that all required tables exist in Pinot
func verifySchemas() error {
	// Check if tables exist via the /tables endpoint (controller only)
	resp, err := http.Get(pinotControllerURL + "/tables")
	if err != nil {
		return fmt.Errorf("failed to get tables list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get tables list: status %d", resp.StatusCode)
	}

	var tablesResp struct {
		Tables []string `json:"tables"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tablesResp); err != nil {
		return fmt.Errorf("failed to decode tables response: %w", err)
	}

	requiredTables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	tableMap := make(map[string]bool)
	for _, table := range tablesResp.Tables {
		tableMap[table] = true
	}

	for _, required := range requiredTables {
		if !tableMap[required] {
			return fmt.Errorf("table %s not found", required)
		}
	}

	return nil
}

// cleanupAll removes all test data from all tables
func cleanupAll() {
	client := pinot.NewClient(pinotBrokerURL)
	ctx := context.Background()

	tables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	for _, table := range tables {
		// Note: This assumes tenant_id 0-1000 are test tenants
		// In production, use a dedicated test database/namespace
		sql := fmt.Sprintf("DELETE FROM %s WHERE tenant_id < 1000", table)
		_, _ = client.Query(ctx, sql)
	}
}

// cleanupTestTenants removes test data for specific tenant IDs used in tests
func cleanupTestTenants() {
	// REALTIME tables don't support DELETE in Pinot, so we can't actually clean them
	// The best we can do is note that old data will persist
	// In a real system, you'd use time-based retention or dedicated test databases
	fmt.Println("Note: REALTIME tables accumulate data - old test data may remain")
}
