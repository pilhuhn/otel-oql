package common

import (
	"fmt"
	"time"
)

// TranslateTimeRange converts a time.Duration to a Clickhouse SQL timestamp filter
// This is shared between PromQL and LogQL
func TranslateTimeRange(duration time.Duration) string {
	millis := duration.Milliseconds()
	return fmt.Sprintf("timestamp >= (toUnixTimestamp(now()) * 1000 - %d)", millis)
}

// TranslateSinceTimestamp converts a timestamp to a Clickhouse SQL filter
func TranslateSinceTimestamp(timestamp time.Time) string {
	millis := timestamp.UnixMilli()
	return fmt.Sprintf("timestamp >= %d", millis)
}

// TranslateBetweenTimestamps converts a time range to a Clickhouse SQL filter
func TranslateBetweenTimestamps(start, end time.Time) string {
	startMillis := start.UnixMilli()
	endMillis := end.UnixMilli()
	return fmt.Sprintf("timestamp >= %d AND timestamp <= %d", startMillis, endMillis)
}
