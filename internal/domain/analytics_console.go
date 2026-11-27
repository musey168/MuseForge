package domain

import "time"

// UsageConsoleSnapshot is the read model of the usage console: the tokens and
// the cost of the work reported from outside the platform, and nothing else.
// The platform runs no work of its own, so there is no run quality to report.
type UsageConsoleSnapshot struct {
	Summary    UsageConsoleSummary      `json:"summary"`
	Trend      []UsageConsoleTrendPoint `json:"trend"`
	Models     []UsageConsoleBreakdown   `json:"models"`
	Providers  []UsageConsoleBreakdown   `json:"providers"`
	Workflows  []UsageConsoleWorkflow    `json:"workflows"`
	Executions []UsageConsoleExecution   `json:"executions"`
	// Empty is true when the scope holds no reported execution, so the
	// interface can say so instead of showing a page of zeros.
	Empty       bool      `json:"empty"`
	TimeZone    string    `json:"timeZone"`
	Grain       string    `json:"grain"`
	GeneratedAt time.Time `json:"generatedAt"`
}

// UsageConsoleSummary is the console's headline numbers for the chosen period
// and scope.
type UsageConsoleSummary struct {
	// Executions is the number of reported executions in scope.
	Executions int `json:"executions"`
	// InputTokens includes the cache tokens the reporter counted as input.
	InputTokens      int64 `json:"inputTokens"`
	OutputTokens     int64 `json:"outputTokens"`
	CacheReadTokens  int64 `json:"cacheReadTokens"`
	CacheWriteTokens int64 `json:"cacheWriteTokens"`
	// TotalTokens counts each token once: the reported input already contains
	// the cache tokens, so the total is input + output.
	TotalTokens int64 `json:"totalTokens"`
	// CacheHitRate = ΣCacheRead ÷ ΣInput × 100: the reported input includes
	// the cache tokens, so the denominator is the reported input alone.
	CacheHitRate float64 `json:"cacheHitRate"`
	// TotalCostUSD is the sum of every known cost; TotalCostCNY is the same
	// sum in the reader's currency.
	TotalCostUSD float64 `json:"totalCostUsd"`
	TotalCostCNY float64 `json:"totalCostCny"`
	// CostCurrency is the currency the reader sees, CostUSDToCNY the rate the
	// platform publishes for it.
	CostCurrency string  `json:"costCurrency"`
	CostUSDToCNY float64 `json:"usdToCny"`
	// CostReportedUSD and CostEstimatedUSD split the total by how the cost was
	// known: reported by the reporter, or priced from the published table.
	CostReportedUSD  float64 `json:"costReportedUsd"`
	CostEstimatedUSD float64 `json:"costEstimatedUsd"`
	// ReportedExecutions counts executions whose cost the reporter reported.
	// EstimatedExecutions counts executions priced from the published table:
	// they add to the total but are not reported executions.
	ReportedExecutions    int `json:"reportedExecutions"`
	EstimatedExecutions   int `json:"estimatedExecutions"`
	UnknownCostExecutions int `json:"unknownCostExecutions"`
	// CostCoveragePct = (reported + estimated) ÷ executions × 100.
	CostCoveragePct float64 `json:"costCoveragePct"`
	// MalformedReports counts the reports the ingest refused as malformed.
	MalformedReports int64 `json:"malformedReports"`
	// SkippedExecutions counts the stored reports the console could not read.
	SkippedExecutions int `json:"skippedExecutions"`
	GeneratedAt       time.Time `json:"generatedAt"`
}

// UsageConsoleTrendPoint is one day, week or month bucket of the trend.
type UsageConsoleTrendPoint struct {
	Date             string  `json:"date"`
	Executions       int     `json:"executions"`
	Tokens           int64   `json:"tokens"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	CostUSD          float64 `json:"costUsd"`
	CostCNY          float64 `json:"costCny"`
}

// UsageConsoleBreakdown is one model or one provider with its share of the
// tokens and of the cost.
type UsageConsoleBreakdown struct {
	Key              string  `json:"key"`
	Name             string  `json:"name"`
	Executions       int     `json:"executions"`
	Tokens           int64   `json:"tokens"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	CostUSD          float64 `json:"costUsd"`
	CostCNY          float64 `json:"costCny"`
	TokenSharePct    float64 `json:"tokenSharePct"`
	CostSharePct     float64 `json:"costSharePct"`
}

// UsageConsoleWorkflow is one reported workflow the scope covers, so the
// interface can scope the console by workflow.
type UsageConsoleWorkflow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UsageConsoleUsage is one model entry of a reported execution.
type UsageConsoleUsage struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CacheReadTokens  int64   `json:"cacheReadTokens"`
	CacheWriteTokens int64   `json:"cacheWriteTokens"`
	ReasoningTokens  int64   `json:"reasoningTokens"`
	CostUSD          float64 `json:"costUsd"`
	// CostStatus is reported, estimated or unknown: an unknown cost is never
	// counted as a known zero.
	CostStatus string `json:"costStatus"`
}

// UsageConsoleExecution is one reported execution, newest first.
type UsageConsoleExecution struct {
	ID                  string              `json:"id"`
	ExternalExecutionID string              `json:"externalExecutionId"`
	WorkflowID          string              `json:"workflowId"`
	WorkflowName        string              `json:"workflowName"`
	Status              string              `json:"status"`
	StartedAt           time.Time           `json:"startedAt"`
	FinishedAt          time.Time           `json:"finishedAt"`
	DurationMs          int64               `json:"durationMs"`
	Tokens              int64               `json:"tokens"`
	InputTokens         int64               `json:"inputTokens"`
	OutputTokens        int64               `json:"outputTokens"`
	CacheReadTokens     int64               `json:"cacheReadTokens"`
	CacheWriteTokens    int64               `json:"cacheWriteTokens"`
	CostUSD             float64             `json:"costUsd"`
	CostCNY             float64             `json:"costCny"`
	// CostStatus is the execution's cost status: reported, estimated or
	// unknown.
	CostStatus string              `json:"costStatus"`
	Usage      []UsageConsoleUsage `json:"usage"`
}
