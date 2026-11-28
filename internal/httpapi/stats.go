package httpapi

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"agentflow/internal/domain"

	"github.com/gin-gonic/gin"
)

// statsHandlers serves the statistics page's picture of the platform's own
// data. It is a read view: it aggregates the personal build's small tables in
// Go rather than owning a roll-up table that would need its own migrations.
type statsHandlers struct {
	*deps
}

const (
	statsDefaultSince = 30 * 24 * time.Hour
	statsMaxTags      = 12
)

// overview answers GET /api/stats/overview.
func (h *statsHandlers) overview(c *gin.Context) {
	grain := strings.ToLower(strings.TrimSpace(c.DefaultQuery("grain", "day")))
	switch grain {
	case "day", "week", "month":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "grain 必须为 day、week 或 month"})
		return
	}
	since := time.Now().UTC().Add(-statsDefaultSince)
	if raw := strings.TrimSpace(c.Query("since")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 || parsed > 5*365*24*time.Hour {
			c.JSON(http.StatusBadRequest, gin.H{"error": "since 必须是 1h 到 43800h 之间的时长"})
			return
		}
		since = time.Now().UTC().Add(-parsed)
	}
	projectID := strings.TrimSpace(c.Query("projectId"))
	if projectID != "" {
		if _, ok := h.repo.Project(projectID); !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "项目不存在"})
			return
		}
	}
	c.JSON(http.StatusOK, aggregatePlatformStats(platformStatsInput{
		Projects:     h.repo.Projects(),
		Issues:       h.repo.Issues(),
		Repositories: h.repo.Repositories(),
		Machines:     h.repo.Runners(),
	}, projectID, grain, since, domain.TrendLocation(c.Query("tz")), time.Now().UTC()))
}

// platformStatsInput is everything the statistics page reads, gathered so the
// aggregation itself stays a pure function.
type platformStatsInput struct {
	Projects     []domain.Project
	Issues       []domain.Issue
	Repositories []domain.Repository
	Machines     []domain.Runner
}

func aggregatePlatformStats(in platformStatsInput, projectID, grain string, since time.Time, zone *time.Location, now time.Time) domain.PlatformStats {
	stats := domain.PlatformStats{
		Projects:    len(in.Projects),
		Grain:       grain,
		GeneratedAt: now,
		Requirements: domain.RequirementStats{
			ByStatus:   map[string]int{},
			BySource:   map[string]int{},
			ByPriority: map[string]int{},
			Tags:       []domain.TagCount{},
		},
		Repositories: domain.RepositoryStats{ByProvider: map[string]int{}},
	}

	tags := map[string]int{}
	created := map[string]int{}
	closed := map[string]int{}
	for _, issue := range in.Issues {
		if projectID != "" && issue.ProjectID != projectID {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(issue.Status))
		stats.Requirements.Total++
		stats.Requirements.ByStatus[status]++
		if domain.RequirementClosed(status) {
			stats.Requirements.Closed++
		} else {
			stats.Requirements.Open++
		}
		source := strings.ToLower(strings.TrimSpace(issue.Source))
		if source == "" {
			source = "local"
		}
		stats.Requirements.BySource[source]++
		priority := strings.ToLower(strings.TrimSpace(issue.Priority))
		if priority == "" {
			priority = "medium"
		}
		stats.Requirements.ByPriority[priority]++
		for _, tag := range issue.Tags {
			if tag = strings.TrimSpace(tag); tag != "" {
				tags[tag]++
			}
		}
		if !issue.CreatedAt.IsZero() && !issue.CreatedAt.Before(since) {
			created[domain.TrendBucket(issue.CreatedAt.In(zone), grain)]++
		}
		if !issue.CompletedAt.IsZero() && !issue.CompletedAt.Before(since) {
			closed[domain.TrendBucket(issue.CompletedAt.In(zone), grain)]++
		}
	}
	stats.Requirements.Tags = sortedTagCounts(tags)
	for _, bucket := range trendBuckets(since, now, zone, grain) {
		stats.Trend = append(stats.Trend, domain.StatsPoint{
			Bucket:  bucket,
			Created: created[bucket],
			Closed:  closed[bucket],
		})
	}

	for _, repository := range in.Repositories {
		if projectID != "" && repository.ProjectID != projectID {
			continue
		}
		stats.Repositories.Total++
		provider := strings.ToLower(strings.TrimSpace(repository.Provider))
		if provider == "" {
			provider = "other"
		}
		stats.Repositories.ByProvider[provider]++
	}

	for _, machine := range in.Machines {
		stats.Machines.Total++
		if machine.Status == domain.RunnerStatusOnline {
			stats.Machines.Connected++
		}
	}

	stats.Empty = stats.Requirements.Total == 0 && stats.Repositories.Total == 0 && stats.Machines.Total == 0
	return stats
}

// trendBuckets lists every bucket between since and now in order, so the chart
// shows a quiet week as a gap rather than skipping it.
func trendBuckets(since, now time.Time, zone *time.Location, grain string) []string {
	start := since.In(zone)
	switch grain {
	case "month":
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, zone)
	case "week":
		for start.Weekday() != time.Monday {
			start = start.AddDate(0, 0, -1)
		}
	default:
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, zone)
	}
	var buckets []string
	for at := start; !at.After(now.In(zone)); {
		buckets = append(buckets, domain.TrendBucket(at, grain))
		switch grain {
		case "month":
			at = at.AddDate(0, 1, 0)
		case "week":
			at = at.AddDate(0, 0, 7)
		default:
			at = at.AddDate(0, 0, 1)
		}
	}
	return buckets
}

func sortedTagCounts(tags map[string]int) []domain.TagCount {
	out := make([]domain.TagCount, 0, len(tags))
	for tag, count := range tags {
		out = append(out, domain.TagCount{Tag: tag, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	if len(out) > statsMaxTags {
		out = out[:statsMaxTags]
	}
	return out
}