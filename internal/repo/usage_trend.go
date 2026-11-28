package repo

import (
	"time"

	"agentflow/internal/domain"
)

// analyticsWeekHourLocation resolves the reporting zone; an unknown or empty
// zone falls back to Asia/Shanghai, which is the platform's reporting default.
func analyticsWeekHourLocation(name string) *time.Location {
	return domain.TrendLocation(name)
}

// analyticsTrendBucket names the day, week or month a moment falls into.
func analyticsTrendBucket(at time.Time, grain string) string {
	return domain.TrendBucket(at, grain)
}