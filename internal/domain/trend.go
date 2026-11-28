package domain

import (
	"fmt"
	"strings"
	"time"
)

// TrendLocation resolves the reporting zone a trend is bucketed in. An unknown
// or empty zone falls back to Asia/Shanghai, the platform's reporting default.
func TrendLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		name = "Asia/Shanghai"
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("Asia/Shanghai", 8*3600)
}

// TrendBucket names the day, week or month a moment falls into. The usage
// console and the platform trend both bucket with it, so the two charts never
// disagree about which bar a moment belongs to.
func TrendBucket(at time.Time, grain string) string {
	switch strings.ToLower(strings.TrimSpace(grain)) {
	case "week":
		year, week := at.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "month", "all":
		return at.Format("2006-01")
	default:
		return at.Format("2006-01-02")
	}
}