package domain

import "time"

// PlatformStats is the statistics page's picture of the platform's own data:
// projects, requirements, repositories and machines. The personal build keeps
// these tables small, so the handler aggregates them in Go instead of running a
// SQL roll-up that would need its own migrations.
type PlatformStats struct {
	Projects     int                     `json:"projects"`
	Requirements RequirementStats        `json:"requirements"`
	Trend        []StatsPoint            `json:"trend"`
	Repositories RepositoryStats         `json:"repositories"`
	Machines     MachineStats            `json:"machines"`
	Empty        bool                    `json:"empty"`
	Grain        string                  `json:"grain"`
	Labels       map[string]string       `json:"labels,omitempty"`
	GeneratedAt  time.Time               `json:"generatedAt"`
}

// RequirementStats answers "what do I owe myself" without asking the list.
type RequirementStats struct {
	Total      int            `json:"total"`
	Open       int            `json:"open"`
	Closed     int            `json:"closed"`
	ByStatus   map[string]int `json:"byStatus"`
	BySource   map[string]int `json:"bySource"`
	ByPriority map[string]int `json:"byPriority"`
	Tags       []TagCount     `json:"tags"`
}

// TagCount is one label and how many requirements carry it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// StatsPoint is one bucket of the created/closed trend.
type StatsPoint struct {
	Bucket  string `json:"bucket"`
	Created int    `json:"created"`
	Closed  int    `json:"closed"`
}

// RepositoryStats counts the connected repositories by their provider.
type RepositoryStats struct {
	Total      int            `json:"total"`
	ByProvider map[string]int `json:"byProvider"`
}

// MachineStats counts the registered development machines.
type MachineStats struct {
	Total     int `json:"total"`
	Connected int `json:"connected"`
}

// RequirementClosed reports whether a status counts as finished. It mirrors the
// console's issueOpen so the page and the statistics never disagree.
func RequirementClosed(status string) bool {
	switch status {
	case "done", "closed", "cancelled":
		return true
	default:
		return false
	}
}