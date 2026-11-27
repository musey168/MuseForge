package repo

import (
	"agentflow/internal/domain"
)

// analyticsInsightPricedUsage prices one reported usage entry. An explicitly
// reported cost wins; an absent one is priced from the published table and
// marked estimated.
func analyticsInsightPricedUsage(u domain.InsightUsage) domain.TokenUsage {
	usage := domain.TokenUsage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.InputTokens + u.OutputTokens, CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens, ReasoningTokens: u.ReasoningTokens, CostStatus: u.CostStatus}
	if u.CostUSD != nil {
		usage.CostUSD = *u.CostUSD
	}
	return domain.AnalyticsPricedUsage(u.Model, usage, true)
}
