package integration

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/clickhouse"
)

// TestMain handles setup and teardown for integration tests
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("⚠️  skip integration tests: -short")
		os.Exit(0)
	}

	// Check if Clickhouse is running
	if !isClickhouseAvailable() {
		if os.Getenv("REQUIRE_INTEGRATION") == "1" {
			fmt.Println("❌ Clickhouse is not running or not accessible at " + clickhouseURL)
			fmt.Println("Start Clickhouse with: podman-compose up -d")
			fmt.Println("Then ensure schemas are created: ./otel-oql setup-schema --clickhouse-url=" + clickhouseURL)
			os.Exit(1)
		}
		fmt.Println("⚠️  skip integration tests: Clickhouse not reachable at " + clickhouseURL)
		fmt.Println("Start Clickhouse with: podman-compose up -d, then: go test ./pkg/integration/... -count=1")
		fmt.Println("Or set REQUIRE_INTEGRATION=1 to fail when Clickhouse is down (e.g. CI with Clickhouse).")
		os.Exit(0)
	}

	fmt.Println("✅ Clickhouse is running and accessible")

	// Check if OTEL-OQL service is running
	if !isOtelOQLAvailable() {
		fmt.Println("⚠️  OTEL-OQL service is not running")
		fmt.Println("Start service with: ./otel-oql --test-mode --clickhouse-url=" + clickhouseURL)
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

// isClickhouseAvailable checks if Clickhouse is running and accessible
func isClickhouseAvailable() bool {
	resp, err := http.Get(clickhouseURL + "/ping")
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

// verifySchemas checks that all required tables exist in Clickhouse
func verifySchemas() error {
	client := clickhouse.NewClient(clickhouseURL)
	ctx := context.Background()

	requiredTables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	for _, table := range requiredTables {
		sql := fmt.Sprintf("SELECT count() FROM %s LIMIT 1", table)
		_, err := client.Query(ctx, sql)
		if err != nil {
			return fmt.Errorf("table %s not found or not queryable: %w", table, err)
		}
	}
	return nil
}

// cleanupAll removes all test data from all tables
func cleanupAll() {
	client := clickhouse.NewClient(clickhouseURL)
	ctx := context.Background()

	tables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	for _, table := range tables {
		sql := fmt.Sprintf("DELETE FROM %s WHERE tenant_id < 1000", table)
		_, _ = client.Query(ctx, sql)
	}
}

// cleanupTestTenants removes test data for specific tenant IDs used in tests
func cleanupTestTenants() {
	client := clickhouse.NewClient(clickhouseURL)
	ctx := context.Background()

	tables := []string{"otel_spans", "otel_metrics", "otel_logs"}
	for _, table := range tables {
		sql := fmt.Sprintf("ALTER TABLE %s DELETE WHERE tenant_id < 1000", table)
		_, _ = client.Query(ctx, sql)
	}
}
