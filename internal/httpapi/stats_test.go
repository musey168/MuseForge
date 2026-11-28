package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentflow/internal/domain"
)

type platformStatsResponse struct {
	Projects     int `json:"projects"`
	Requirements struct {
		Total      int            `json:"total"`
		Open       int            `json:"open"`
		Closed     int            `json:"closed"`
		ByStatus   map[string]int `json:"byStatus"`
		BySource   map[string]int `json:"bySource"`
		ByPriority map[string]int `json:"byPriority"`
		Tags       []struct {
			Tag   string `json:"tag"`
			Count int    `json:"count"`
		} `json:"tags"`
	} `json:"requirements"`
	Trend []struct {
		Bucket  string `json:"bucket"`
		Created int    `json:"created"`
		Closed  int    `json:"closed"`
	} `json:"trend"`
	Repositories struct {
		Total      int            `json:"total"`
		ByProvider map[string]int `json:"byProvider"`
	} `json:"repositories"`
	Machines struct {
		Total     int `json:"total"`
		Connected int `json:"connected"`
	} `json:"machines"`
	Empty bool `json:"empty"`
	Grain string `json:"grain"`
}

func readPlatformStats(t *testing.T, h http.Handler, url string) (platformStatsResponse, int) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	var body platformStatsResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode stats: %v (%s)", err, rec.Body.String())
		}
	}
	return body, rec.Code
}

// The aggregation is a pure function, so the counts are tested against issue
// values the store would never let a caller write (historic CreatedAt, a
// CompletedAt from last week).
func TestAggregatePlatformStatsCountsAndTrend(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	zone := time.UTC
	in := platformStatsInput{
		Projects: []domain.Project{{ID: "p-one"}, {ID: "p-two"}},
		Issues: []domain.Issue{
			{ID: "i-1", ProjectID: "p-one", Status: "todo", Priority: "high", Source: "local", Tags: []string{"接口"}, CreatedAt: now.Add(-48 * time.Hour)},
			{ID: "i-2", ProjectID: "p-one", Status: "done", Priority: "low", Source: "github", Tags: []string{"接口", "前端"}, CreatedAt: now.Add(-72 * time.Hour), CompletedAt: now.Add(-24 * time.Hour)},
			{ID: "i-3", ProjectID: "p-two", Status: "closed", Priority: "", Source: "gitlab", CreatedAt: now.Add(-240 * time.Hour)},
			{ID: "i-4", ProjectID: "p-two", Status: "cancelled", Source: "", CreatedAt: now.Add(-45 * 24 * time.Hour)},
		},
		Repositories: []domain.Repository{
			{ID: "r-1", ProjectID: "p-one", Provider: "https"},
			{ID: "r-2", ProjectID: "p-two", Provider: "gitlab"},
		},
		Machines: []domain.Runner{
			{ID: "m-1", Status: domain.RunnerStatusOnline},
			{ID: "m-2", Status: "offline"},
		},
	}

	all := aggregatePlatformStats(in, "", "day", now.Add(-30*24*time.Hour), zone, now)
	if all.Projects != 2 || all.Requirements.Total != 4 {
		t.Fatalf("totals: %+v", all)
	}
	// done, closed and cancelled are finished; todo is not.
	if all.Requirements.Open != 1 || all.Requirements.Closed != 3 {
		t.Fatalf("open/closed: %+v", all.Requirements)
	}
	// An empty source is a requirement written here, so it counts as local.
	if all.Requirements.BySource["local"] != 2 || all.Requirements.BySource["github"] != 1 || all.Requirements.BySource["gitlab"] != 1 {
		t.Fatalf("source split: %v", all.Requirements.BySource)
	}
	if all.Requirements.ByPriority["medium"] != 2 {
		t.Fatalf("an empty priority counts as medium: %v", all.Requirements.ByPriority)
	}
	if len(all.Requirements.Tags) != 2 || all.Requirements.Tags[0].Tag != "接口" || all.Requirements.Tags[0].Count != 2 {
		t.Fatalf("tags: %+v", all.Requirements.Tags)
	}
	if all.Repositories.Total != 2 || all.Repositories.ByProvider["https"] != 1 || all.Repositories.ByProvider["gitlab"] != 1 {
		t.Fatalf("repositories: %+v", all.Repositories)
	}
	if all.Machines.Total != 2 || all.Machines.Connected != 1 {
		t.Fatalf("machines: %+v", all.Machines)
	}
	if len(all.Trend) != 31 {
		t.Fatalf("a 30 day window closes with 31 buckets, got %d", len(all.Trend))
	}
	if all.Trend[len(all.Trend)-1].Bucket != "2026-11-28" || all.Trend[len(all.Trend)-1].Created != 0 {
		t.Fatalf("today's bucket: %+v", all.Trend[len(all.Trend)-1])
	}
	// The requirement created two days ago and the closure one day ago land in
	// their own buckets, not today's.
	found := map[string]domain.StatsPoint{}
	for _, p := range all.Trend {
		found[p.Bucket] = domain.StatsPoint(p)
	}
	if found["2026-11-26"].Created != 1 || found["2026-11-27"].Closed != 1 || found["2026-11-25"].Created != 1 {
		t.Fatalf("buckets: %+v", found)
	}

	// One project narrows the requirements and repositories, never the project
	// count: that number describes the platform.
	scoped := aggregatePlatformStats(in, "p-two", "day", now.Add(-30*24*time.Hour), zone, now)
	if scoped.Requirements.Total != 2 || scoped.Repositories.Total != 1 || scoped.Projects != 2 {
		t.Fatalf("scoped: %+v", scoped)
	}
}

func TestTrendBucketsFillGaps(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	day := trendBuckets(now.Add(-72*time.Hour), now, time.UTC, "day")
	if len(day) != 4 || day[0] != "2026-11-25" || day[3] != "2026-11-28" {
		t.Fatalf("day buckets: %v", day)
	}
	week := trendBuckets(now.Add(-14*24*time.Hour), now, time.UTC, "week")
	if len(week) != 3 || week[0] != "2026-W46" || week[2] != "2026-W48" {
		t.Fatalf("week buckets: %v", week)
	}
	month := trendBuckets(now.AddDate(0, -2, 0), now, time.UTC, "month")
	if len(month) != 3 || month[0] != "2026-09" || month[2] != "2026-11" {
		t.Fatalf("month buckets: %v", month)
	}
}

func TestPlatformStatsEndpointWiring(t *testing.T) {
	f := newHTTPFixture(t)

	empty, code := readPlatformStats(t, f.h, "/api/stats/overview?tz=UTC")
	if code != http.StatusOK || !empty.Empty {
		t.Fatalf("empty store: code=%d body=%+v", code, empty)
	}

	if err := f.s.PutProject(domain.Project{ID: "p-stats", Key: "ST", Name: "统计"}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.PutIssue(domain.Issue{ID: "i-stats", ProjectID: "p-stats", Number: 1, Title: "一件", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	body, code := readPlatformStats(t, f.h, "/api/stats/overview?projectId=p-stats&tz=UTC")
	if code != http.StatusOK || body.Requirements.Total != 1 || body.Requirements.Open != 1 || body.Empty {
		t.Fatalf("scoped store: code=%d body=%+v", code, body)
	}
	// CreatedAt is stamped by the store, so the created count shows up today.
	if last := body.Trend[len(body.Trend)-1]; last.Created != 1 {
		t.Fatalf("today's created count: %+v", last)
	}

	if _, code := readPlatformStats(t, f.h, "/api/stats/overview?grain=year"); code != http.StatusBadRequest {
		t.Fatalf("grain=year must be rejected, got %d", code)
	}
	if _, code := readPlatformStats(t, f.h, "/api/stats/overview?projectId=missing"); code != http.StatusNotFound {
		t.Fatalf("unknown project must be 404, got %d", code)
	}
	if _, code := readPlatformStats(t, f.h, "/api/stats/overview?since=soon"); code != http.StatusBadRequest {
		t.Fatalf("an unparsable window must be rejected, got %d", code)
	}
}