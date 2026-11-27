package repo

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// analyticsActivity is read-only, including its legacy participant fallback.
// Do not use the history-list helpers here: they may backfill stored metadata.
func (r *AnalyticsRepo) analyticsActivity(filter domain.AnalyticsFilter, issues []domain.Issue, runs []domain.Run, out *domain.AnalyticsSnapshot) error {
	out.ActivityTrend = []domain.AnalyticsActivityPoint{}
	if filter.Origin == "external" {
		return nil
	}
	points := map[string]*domain.AnalyticsActivityPoint{}
	activeMembers := map[int64]bool{}
	add := func(at time.Time, count func(*domain.AnalyticsActivity)) {
		if !analyticsActivityInWindow(at, filter) {
			return
		}
		count(&out.Activity)
		bucketAt := at.UTC()
		if filter.Scope == "dashboard" {
			bucketAt = at.In(analyticsWeekHourLocation(filter.TimeZone))
		}
		key := analyticsTrendBucket(bucketAt, filter.Grain)
		if points[key] == nil {
			points[key] = &domain.AnalyticsActivityPoint{Date: key}
		}
		count(&points[key].AnalyticsActivity)
	}
	byID := map[string]domain.Issue{}
	matchesIssue := func(issue domain.Issue) bool {
		return filter.MatchesProject(issue.ProjectID) &&
			(filter.WorkflowID == "" || issue.WorkflowID == filter.WorkflowID) &&
			(filter.IssueSource == "" || analyticsIssueMatchesSource(issue, filter.IssueSource)) &&
			(!filter.RequirementsOnly || analyticsIsRequirementIssue(issue))
	}
	for _, issue := range issues {
		byID[issue.ID] = issue
		if !matchesIssue(issue) || !analyticsIsRequirementIssue(issue) {
			continue
		}
		if filter.UserID != "" && strconv.FormatInt(issue.AssigneeUserID, 10) != filter.UserID {
			continue
		}
		add(issue.CreatedAt, func(a *domain.AnalyticsActivity) { a.Requirements++ })
	}
	for _, run := range runs {
		issue := byID[strings.TrimSpace(run.ExecutionContext.IssueID)]
		projectID := strings.TrimSpace(run.ExecutionContext.ProjectID)
		if projectID == "" {
			projectID = issue.ProjectID
		}
		if run.IsWorkspace() || run.HistoricalGroup() || run.ExecutionContext.ParentRunID != "" ||
			!analyticsIsRequirementIssue(issue) || !filter.MatchesProject(projectID) ||
			(filter.WorkflowID != "" && run.WorkflowID != filter.WorkflowID) ||
			(filter.IssueSource != "" && !analyticsIssueMatchesSource(issue, filter.IssueSource)) ||
			!analyticsRunMatchesUser(run, filter.UserID, issue) {
			continue
		}
		add(run.CreatedAt, func(a *domain.AnalyticsActivity) { a.SOPRuns++ })
	}
	for _, point := range points {
		out.ActivityTrend = append(out.ActivityTrend, *point)
	}
	sort.Slice(out.ActivityTrend, func(i, j int) bool { return out.ActivityTrend[i].Date < out.ActivityTrend[j].Date })
	if out.Dashboard != nil {
		out.Dashboard.ActiveMembers = len(activeMembers)
	}
	return nil
}

func analyticsActivityInWindow(at time.Time, filter domain.AnalyticsFilter) bool {
	return !at.IsZero() && (filter.From.IsZero() || !at.Before(filter.From)) && (filter.To.IsZero() || !at.After(filter.To))
}
