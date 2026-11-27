package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"agentflow/internal/domain"
)

// ErrInvalidUsageBucket marks a period bucket the console cannot read.
var ErrInvalidUsageBucket = errors.New("bucket 无效")

// UsageConsole aggregates the externally reported executions into the usage
// console read model: tokens, cost, cache hits, the trend, the model and
// provider breakdowns and the executions behind them. The platform runs no
// work of its own, so every number comes from a report.
func (r *InsightsRepo) UsageConsole(filter domain.AnalyticsFilter) (domain.UsageConsoleSnapshot, error) {
	now := time.Now().UTC()
	zone := analyticsWeekHourLocation(filter.TimeZone)
	grain := consoleGrain(filter.Grain)
	from, to := filter.From, filter.To
	if bucket := strings.TrimSpace(filter.Bucket); bucket != "" {
		start, end, ok := consoleBucketRange(bucket, grain, zone)
		if !ok {
			return domain.UsageConsoleSnapshot{}, ErrInvalidUsageBucket
		}
		from, to = start, end
	}

	out := domain.UsageConsoleSnapshot{
		TimeZone:    zone.String(),
		Grain:       grain,
		GeneratedAt: now,
		Summary: domain.UsageConsoleSummary{
			CostCurrency: "CNY",
			CostUSDToCNY: domain.AnalyticsUSDToCNY,
			GeneratedAt:  now,
		},
		Trend:      []domain.UsageConsoleTrendPoint{},
		Models:     []domain.UsageConsoleBreakdown{},
		Providers:  []domain.UsageConsoleBreakdown{},
		Workflows:  []domain.UsageConsoleWorkflow{},
		Executions: []domain.UsageConsoleExecution{},
	}

	rows, err := r.insightExecutionRows(filter, from, to)
	if err != nil {
		return out, err
	}

	models := map[string]*domain.UsageConsoleBreakdown{}
	providers := map[string]*domain.UsageConsoleBreakdown{}
	workflows := map[string]*domain.UsageConsoleWorkflow{}
	trends := map[string]*domain.UsageConsoleTrendPoint{}

	var totalTokens, inputTokens, outputTokens, cacheRead, cacheWrite int64
	var totalCost, reportedCost, estimatedCost float64

	for _, row := range rows {
		var execution domain.InsightExecution
		if err := json.Unmarshal(row.Payload, &execution); err != nil {
			// A stored report the console cannot read is skipped and counted;
			// it never breaks the rest of the console.
			out.Summary.SkippedExecutions++
			continue
		}
		if filter.WorkflowID != "" && !strings.EqualFold(strings.TrimSpace(filter.WorkflowID), strings.TrimSpace(execution.Workflow.ID)) {
			continue
		}

		view := domain.UsageConsoleExecution{
			ID:                  execution.ID,
			ExternalExecutionID: execution.ExternalExecutionID,
			WorkflowID:          execution.Workflow.ID,
			WorkflowName:        execution.Workflow.Name,
			Status:              execution.Status,
			StartedAt:           execution.StartedAt,
			FinishedAt:          execution.FinishedAt,
			Usage:               []domain.UsageConsoleUsage{},
		}
		if execution.ExecutionDurationMs != nil {
			// The reporter's own duration is the sum of the active node time;
			// the wall-clock span is only a fallback.
			view.DurationMs = *execution.ExecutionDurationMs
		} else if !execution.StartedAt.IsZero() && !execution.FinishedAt.IsZero() {
			view.DurationMs = execution.FinishedAt.Sub(execution.StartedAt).Milliseconds()
		}

		reported, estimated, unknown := false, false, false
		for _, usage := range execution.Usage {
			cost, status := consoleUsageCost(usage)
			view.Usage = append(view.Usage, domain.UsageConsoleUsage{
				Provider:         usage.Provider,
				Model:            usage.Model,
				InputTokens:      usage.InputTokens,
				OutputTokens:     usage.OutputTokens,
				CacheReadTokens:  usage.CacheReadTokens,
				CacheWriteTokens: usage.CacheWriteTokens,
				ReasoningTokens:  usage.ReasoningTokens,
				CostUSD:          cost,
				CostStatus:       status,
			})
			tokens := usage.InputTokens + usage.OutputTokens
			view.Tokens += tokens
			view.InputTokens += usage.InputTokens
			view.OutputTokens += usage.OutputTokens
			view.CacheReadTokens += usage.CacheReadTokens
			view.CacheWriteTokens += usage.CacheWriteTokens
			view.CostUSD += cost

			switch status {
			case "reported":
				reported = true
				reportedCost += cost
			case "estimated":
				estimated = true
				estimatedCost += cost
			default:
				unknown = true
			}

			model := consoleBreakdownEntry(models, usage.Model, usage.Model)
			model.Executions++
			model.Tokens += tokens
			model.InputTokens += usage.InputTokens
			model.OutputTokens += usage.OutputTokens
			model.CacheReadTokens += usage.CacheReadTokens
			model.CacheWriteTokens += usage.CacheWriteTokens
			model.CostUSD += cost

			provider := consoleBreakdownEntry(providers, usage.Provider, usage.Provider)
			provider.Executions++
			provider.Tokens += tokens
			provider.InputTokens += usage.InputTokens
			provider.OutputTokens += usage.OutputTokens
			provider.CacheReadTokens += usage.CacheReadTokens
			provider.CacheWriteTokens += usage.CacheWriteTokens
			provider.CostUSD += cost
		}

		switch {
		case unknown:
			view.CostStatus = "unknown"
		case estimated:
			view.CostStatus = "estimated"
		case reported:
			view.CostStatus = "reported"
		default:
			view.CostStatus = "unknown"
		}
		view.CostCNY = view.CostUSD * domain.AnalyticsUSDToCNY

		totalTokens += view.Tokens
		inputTokens += view.InputTokens
		outputTokens += view.OutputTokens
		cacheRead += view.CacheReadTokens
		cacheWrite += view.CacheWriteTokens
		totalCost += view.CostUSD
		out.Summary.Executions++
		switch view.CostStatus {
		case "reported":
			out.Summary.ReportedExecutions++
		case "estimated":
			out.Summary.EstimatedExecutions++
		default:
			out.Summary.UnknownCostExecutions++
		}

		if id := strings.TrimSpace(execution.Workflow.ID); id != "" {
			workflow := workflows[id]
			if workflow == nil {
				workflow = &domain.UsageConsoleWorkflow{ID: id, Name: execution.Workflow.Name}
				workflows[id] = workflow
			}
			if workflow.Name == "" {
				workflow.Name = execution.Workflow.Name
			}
		}

		bucket := analyticsTrendBucket(execution.FinishedAt.In(zone), grain)
		point := trends[bucket]
		if point == nil {
			point = &domain.UsageConsoleTrendPoint{Date: bucket}
			trends[bucket] = point
		}
		point.Executions++
		point.Tokens += view.Tokens
		point.InputTokens += view.InputTokens
		point.OutputTokens += view.OutputTokens
		point.CacheReadTokens += view.CacheReadTokens
		point.CacheWriteTokens += view.CacheWriteTokens
		point.CostUSD += view.CostUSD

		out.Executions = append(out.Executions, view)
	}

	// Newest first: the executions behind the totals, drillable one by one.
	sort.SliceStable(out.Executions, func(i, j int) bool {
		return out.Executions[i].FinishedAt.After(out.Executions[j].FinishedAt)
	})

	out.Summary.TotalTokens = totalTokens
	out.Summary.InputTokens = inputTokens
	out.Summary.OutputTokens = outputTokens
	out.Summary.CacheReadTokens = cacheRead
	out.Summary.CacheWriteTokens = cacheWrite
	out.Summary.TotalCostUSD = totalCost
	out.Summary.CostReportedUSD = reportedCost
	out.Summary.CostEstimatedUSD = estimatedCost
	out.Summary.TotalCostCNY = totalCost * domain.AnalyticsUSDToCNY
	if inputTokens > 0 {
		// The reported input includes the cache tokens, so the cache-hit rate is
		// cache-read over the reported input.
		out.Summary.CacheHitRate = float64(cacheRead) * 100 / float64(inputTokens)
	}
	if out.Summary.Executions > 0 {
		known := out.Summary.ReportedExecutions + out.Summary.EstimatedExecutions
		out.Summary.CostCoveragePct = float64(known) * 100 / float64(out.Summary.Executions)
	}

	for _, point := range trends {
		point.CostCNY = point.CostUSD * domain.AnalyticsUSDToCNY
		out.Trend = append(out.Trend, *point)
	}
	sort.Slice(out.Trend, func(i, j int) bool { return out.Trend[i].Date < out.Trend[j].Date })

	out.Models = consoleBreakdowns(models, totalTokens, totalCost)
	out.Providers = consoleBreakdowns(providers, totalTokens, totalCost)
	for _, workflow := range workflows {
		out.Workflows = append(out.Workflows, *workflow)
	}
	sort.Slice(out.Workflows, func(i, j int) bool { return out.Workflows[i].ID < out.Workflows[j].ID })
	out.Empty = out.Summary.Executions == 0

	rejected, err := r.RejectedInsightReports(filter.ProjectID, filter.ProjectIDs)
	if err != nil {
		return out, err
	}
	out.Summary.MalformedReports = rejected
	return out, nil
}

// insightExecutionRows returns the reported executions of the scope. The user
// scope is the platform user the report was mapped to.
func (r *InsightsRepo) insightExecutionRows(filter domain.AnalyticsFilter, from, to time.Time) ([]insightExecutionRow, error) {
	rows := []insightExecutionRow{}
	q := r.DB.Model(&insightExecutionRow{})
	if filter.ProjectID != "" {
		q = q.Where("project_id = ?", filter.ProjectID)
	}
	if filter.ProjectIDs != nil {
		if len(filter.ProjectIDs) == 0 {
			return rows, nil
		}
		q = q.Where("project_id IN ?", filter.ProjectIDs)
	}
	if user := strings.TrimSpace(filter.UserID); user != "" {
		if id, err := strconv.ParseInt(user, 10, 64); err == nil {
			q = q.Where("user_id = ?", id)
		}
	}
	if !from.IsZero() {
		q = q.Where("finished_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("finished_at <= ?", to)
	}
	err := q.Order("finished_at, id").Find(&rows).Error
	return rows, err
}

// consoleUsageCost resolves the cost of one reported usage entry. An
// explicitly reported cost wins and is used as-is; an absent one is priced
// from the published price table and marked estimated; a cost the reporter
// marked estimated is used as-is but never counted as reported.
func consoleUsageCost(usage domain.InsightUsage) (float64, string) {
	switch strings.ToLower(strings.TrimSpace(usage.CostStatus)) {
	case "reported":
		if usage.CostUSD != nil {
			return *usage.CostUSD, "reported"
		}
	case "estimated":
		if usage.CostUSD != nil {
			return *usage.CostUSD, "estimated"
		}
	}
	priced := analyticsInsightPricedUsage(usage)
	if priced.CostStatus != "estimated" {
		// The price table has no tariff for this model: the cost stays unknown
		// instead of silently becoming zero.
		return 0, "unknown"
	}
	return priced.CostUSD, "estimated"
}

func consoleBreakdownEntry(entries map[string]*domain.UsageConsoleBreakdown, key, name string) *domain.UsageConsoleBreakdown {
	entry := entries[key]
	if entry == nil {
		entry = &domain.UsageConsoleBreakdown{Key: key, Name: name}
		entries[key] = entry
	}
	return entry
}

func consoleBreakdowns(entries map[string]*domain.UsageConsoleBreakdown, totalTokens int64, totalCost float64) []domain.UsageConsoleBreakdown {
	out := make([]domain.UsageConsoleBreakdown, 0, len(entries))
	for _, entry := range entries {
		entry.CostCNY = entry.CostUSD * domain.AnalyticsUSDToCNY
		if totalTokens > 0 {
			entry.TokenSharePct = float64(entry.Tokens) * 100 / float64(totalTokens)
		}
		if totalCost > 0 {
			entry.CostSharePct = entry.CostUSD * 100 / totalCost
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tokens != out[j].Tokens {
			return out[i].Tokens > out[j].Tokens
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// consoleGrain is the trend granularity the console reads: day, week or month.
func consoleGrain(grain string) string {
	switch strings.ToLower(strings.TrimSpace(grain)) {
	case "week":
		return "week"
	case "month":
		return "month"
	default:
		return "day"
	}
}

// consoleBucketRange turns one trend bucket into the period it covers, read in
// the console's own timezone: a day, an ISO week or a month.
func consoleBucketRange(bucket, grain string, zone *time.Location) (time.Time, time.Time, bool) {
	switch grain {
	case "week":
		var year, week int
		if _, err := fmt.Sscanf(bucket, "%d-W%02d", &year, &week); err != nil || week < 1 || week > 53 {
			return time.Time{}, time.Time{}, false
		}
		// The Monday of ISO week 1 is the week of January 4th.
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, zone)
		daysSinceMonday := (int(jan4.Weekday()) + 6) % 7
		monday := jan4.AddDate(0, 0, -daysSinceMonday)
		start := monday.AddDate(0, 0, (week-1)*7)
		return start, start.AddDate(0, 0, 7).Add(-time.Millisecond), true
	case "month":
		start, err := time.ParseInLocation("2006-01", bucket, zone)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		return start, start.AddDate(0, 1, 0).Add(-time.Millisecond), true
	default:
		start, err := time.ParseInLocation("2006-01-02", bucket, zone)
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
		return start, start.AddDate(0, 0, 1).Add(-time.Millisecond), true
	}
}
