package common

import (
	"fmt"
	"time"
)

// TranslateTimeRange converts a time.Duration to a Pinot SQL timestamp filter
// This is shared between PromQL and LogQL
func TranslateTimeRange(duration time.Duration) string {
	millis := duration.Milliseconds()
	return fmt.Sprintf("\"timestamp\" >= (now() - %d)", millis)
}

// TranslateSinceTimestamp converts a timestamp to a Pinot SQL filter
func TranslateSinceTimestamp(timestamp time.Time) string {
	millis := timestamp.UnixMilli()
	return fmt.Sprintf("\"timestamp\" >= %d", millis)
}

// TranslateBetweenTimestamps converts a time range to a Pinot SQL filter
func TranslateBetweenTimestamps(start, end time.Time) string {
	startMillis := start.UnixMilli()
	endMillis := end.UnixMilli()
	return fmt.Sprintf("\"timestamp\" >= %d AND \"timestamp\" <= %d", startMillis, endMillis)
}
