package domain

import (
	"strconv"
	"strings"
	"time"
)

type Issue struct {
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Status         string `json:"status"`                // backlog, todo, in_progress, in_review, done, closed, blocked, cancelled
	CloseSource    string `json:"closeSource,omitempty"` // manual, external; empty on older manually closed issues
	Priority       string `json:"priority"`
	AssigneeType   string `json:"assigneeType,omitempty"` // member, agent
	AssigneeID     string `json:"assigneeId,omitempty"`
	AssigneeUserID int64  `json:"assigneeUserId,omitempty"`
	WorkflowID     string `json:"workflowId,omitempty"`
	// PlaybookID is which named project combo this requirement uses.
	PlaybookID string `json:"playbookId,omitempty"`
	// SOPBindings is an optional per-issue override of the chosen playbook.
	// Nil means inherit that playbook's steps. AssigneeUserID on the issue
	// stays the overall owner / TAPD 跟进人.
	SOPBindings []IssueSOPBinding `json:"sopBindings,omitempty"`
	// SOPMode "none" delivers the requirement without a LOOP (clarified and
	// changed in conversations): no LOOP is bound by default and scheduled LOOPs
	// never claim it. Empty on writes keeps the stored mode; "sop" clears it.
	SOPMode      string `json:"sopMode,omitempty"`
	RepositoryID string `json:"repositoryId,omitempty"`
	// RepositoryIDs lists every repository the issue touches. RepositoryID
	// stays the legacy single-repository field (kept in sync with the first
	// entry for display and external sync).
	RepositoryIDs []string `json:"repositoryIds,omitempty"`
	// BaseBranch is the legacy API key for an existing work branch to continue.
	// Empty creates a new work branch from the repository configured main branch.
	BaseBranch string `json:"baseBranch,omitempty"`
	// RepositoryBranches selects existing work branches by repository ID. When
	// provided, it replaces BaseBranch; an unlisted repository creates a new branch.
	RepositoryBranches map[string]string       `json:"repositoryBranches,omitempty"`
	LatestTaskID       string                  `json:"latestTaskId,omitempty"`
	AutomationState    string                  `json:"automationState,omitempty"` // ignored means scheduled LOOPs must not claim it
	DeliveryState      string                  `json:"deliveryState,omitempty"`   // merged, reopened
	Kind               string                  `json:"kind,omitempty"`            // requirement, issue
	Clarification      ClarificationAssessment `json:"clarification"`
	Source             string                  `json:"source,omitempty"` // local, github, gitlab
	ExternalRef        *ExternalRef            `json:"externalRef,omitempty"`
	// ConversationSessionID is the conversation that identified and created
	// this requirement while changing code directly, without a LOOP.
	ConversationSessionID string    `json:"conversationSessionId,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	// CompletedAt records an observed transition to done/closed, independent of
	// later edits and MR polling. Older completed issues may have no timestamp.
	CompletedAt time.Time `json:"completedAt,omitempty"`
	// Operations is the requirement's own audit trail: create, edit, close, reopen, dispatch.
	Operations []IssueOperation `json:"operations,omitempty"`
}

func (i Issue) Completed() bool {
	switch strings.ToLower(strings.TrimSpace(i.Status)) {
	case "done", "closed", "completed":
		return true
	}
	return false
}

// IssueOperation is one user-visible action on a requirement.
type IssueOperation struct {
	At          time.Time `json:"at"`
	Actor       string    `json:"actor,omitempty"`
	ActorUserID int64     `json:"actorUserId,omitempty"`
	// ActorChannel and ActorSender name the IM identity that asked for the
	// action when it came from a chat Bot: the channel kind (wecom, feishu,
	// dingtalk) and the sender ID in that channel. ActorSenderName snapshots
	// the display name known for that sender at the time of the action, when
	// the channel reports one. Actor stays the platform account the sender is
	// bound to and the one whose permissions were used. All three are empty
	// for actions taken on the platform itself.
	ActorChannel    string `json:"actorChannel,omitempty"`
	ActorSender     string `json:"actorSender,omitempty"`
	ActorSenderName string `json:"actorSenderName,omitempty"`
	Action          string `json:"action"`
	Detail          string `json:"detail,omitempty"`
}

// AppendOperation records an action and keeps the newest 200 entries.
func (i *Issue) AppendOperation(op IssueOperation) {
	if i == nil {
		return
	}
	op.Action = strings.TrimSpace(op.Action)
	if op.Action == "" {
		return
	}
	if op.At.IsZero() {
		op.At = time.Now().UTC()
	}
	i.Operations = append(i.Operations, op)
	const maxOps = 200
	if len(i.Operations) > maxOps {
		i.Operations = append([]IssueOperation(nil), i.Operations[len(i.Operations)-maxOps:]...)
	}
}

// EffectiveRepositoryIDs returns the deduplicated, non-empty list of
// repositories bound to the issue. RepositoryIDs is authoritative;
// RepositoryID (the legacy single-repository field) is appended as fallback.
func (i Issue) EffectiveRepositoryIDs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(i.RepositoryIDs)+1)
	for _, id := range append(append([]string(nil), i.RepositoryIDs...), i.RepositoryID) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// WorkBranchForRepository resolves the per-repository selection, falling back
// to the legacy branch only for a single repository (including project defaults).
func (i Issue) WorkBranchForRepository(repositoryID string) string {
	if i.RepositoryBranches != nil {
		return strings.TrimSpace(i.RepositoryBranches[repositoryID])
	}
	ids := i.EffectiveRepositoryIDs()
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == repositoryID) {
		return strings.TrimSpace(i.BaseBranch)
	}
	return ""
}

type ClarificationAssessment struct {
	Summary              string                  `json:"summary,omitempty"`
	Status               string                  `json:"status,omitempty"` // analyzing, in_progress, ready, needs_user
	History              []ClarificationMessage  `json:"history,omitempty"`
	Questions            []ClarificationQuestion `json:"questions,omitempty"`
	SessionID            string                  `json:"sessionId,omitempty"`
	SessionUserID        int64                   `json:"sessionUserId,omitempty"`
	WriteBackStatus      string                  `json:"writeBackStatus,omitempty"` // pending, succeeded, failed, skipped
	WriteBackError       string                  `json:"writeBackError,omitempty"`
	WriteBackFingerprint string                  `json:"writeBackFingerprint,omitempty"`
	WriteBackAt          time.Time               `json:"writeBackAt,omitempty"`
	// StartedAt is the first observed start of clarification, including
	// pre-issue composer analysis when its server timestamp is carried over.
	StartedAt time.Time `json:"startedAt,omitempty"`
}

type ClarificationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ClarificationQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	MultiSelect bool     `json:"multiSelect,omitempty"`
	FreeText    bool     `json:"freeText,omitempty"`
}

// ExternalRef links a local Issue to an external issue in GitHub or GitLab.
type ExternalRef struct {
	// SourceContentHash records the last imported body so provider adapters can
	// distinguish a remote edit from a locally clarified requirement.
	SourceContentHash string    `json:"sourceContentHash,omitempty"`
	Source            string    `json:"source"`        // github, gitlab
	ExternalID        string    `json:"externalId"`    // provider issue id
	ExternalIID       int       `json:"externalIid"`   // provider issue number
	ExternalURL       string    `json:"externalUrl"`   // clickable URL
	ExternalState     string    `json:"externalState"` // opened/closed on the provider
	ExternalType      string    `json:"externalType"`  // issue, bug, task
	SyncedAt          time.Time `json:"syncedAt"`
	// Imported marks a requirement the platform created on the provider.
	Imported bool `json:"imported,omitempty"`
}

// ManuallyClosed distinguishes an explicit local close from a tracker close.
// Before CloseSource existed, only manual closes used Issue.Status=closed.
func (i Issue) ManuallyClosed() bool {
	return strings.EqualFold(strings.TrimSpace(i.Status), "closed") &&
		!strings.EqualFold(strings.TrimSpace(i.CloseSource), "external")
}

// TrackerClosed reads the tracker's raw state so a lagging local status cannot
// make an externally closed work item executable.
func (i Issue) TrackerClosed() bool {
	if i.ExternalRef == nil || (i.Source != "github" && i.Source != "gitlab") {
		return false
	}
	state := strings.ToLower(strings.TrimSpace(i.ExternalRef.ExternalState))
	return state == "closed" || state == "已关闭"
}

// DeliveryLocked reports whether a completed delivery has made this work item final.
// A user can explicitly reopen it to allow a follow-up run.
func (i Issue) DeliveryLocked() bool {
	return strings.EqualFold(strings.TrimSpace(i.DeliveryState), "merged")
}

// DeliveryLockedLabel is the user-facing stage label for a delivery-locked issue.
func (i Issue) DeliveryLockedLabel() string {
	if i.ManuallyClosed() {
		return "关闭"
	}
	return "交付闭环"
}

const issueIdentityMarker = "需求身份（平台权威，禁止忽略）"

// IdentityBlock is the authoritative tracker identity injected into the task
// prompt. Agents must not guess a tracker ID from branch names when this block
// is present.
func (i Issue) IdentityBlock() string {
	id := strings.TrimSpace(i.ID)
	if id == "" {
		return ""
	}
	source := strings.ToLower(strings.TrimSpace(i.Source))
	if source == "" {
		source = "local"
	}
	var b strings.Builder
	b.WriteString(issueIdentityMarker)
	b.WriteString("：\n")
	b.WriteString("- 平台工作项：")
	b.WriteString(id)
	b.WriteString("\n- 来源：")
	switch source {
	case "github":
		b.WriteString("GitHub")
	case "gitlab":
		b.WriteString("GitLab")
	default:
		b.WriteString("本地")
	}
	b.WriteByte('\n')
	ref := i.ExternalRef
	if ref == nil {
		b.WriteString("- 外部 ID：无\n")
		return b.String()
	}
	if kind := strings.TrimSpace(ref.ExternalType); kind != "" {
		b.WriteString("- 类型：")
		b.WriteString(kind)
		b.WriteByte('\n')
	}
	if extID := strings.TrimSpace(ref.ExternalID); extID != "" {
		b.WriteString("- 外部 ID：")
		b.WriteString(extID)
		b.WriteByte('\n')
	}
	if ref.ExternalIID != 0 {
		b.WriteString("- 外部编号：")
		b.WriteString(strconv.Itoa(ref.ExternalIID))
		b.WriteByte('\n')
	}
	if url := strings.TrimSpace(ref.ExternalURL); url != "" {
		b.WriteString("- 链接：")
		b.WriteString(url)
		b.WriteByte('\n')
	}
	return b.String()
}

func (i Issue) HasIdentityBlock(text string) bool {
	return strings.Contains(text, issueIdentityMarker)
}

const (
	IssueSOPNone = "none"
	IssueSOPUse  = "sop"
)

// WithoutSOP reports whether the requirement opted out of LOOP delivery.
func (i Issue) WithoutSOP() bool {
	return strings.EqualFold(strings.TrimSpace(i.SOPMode), IssueSOPNone)
}

func (i Issue) ExecutionBlockedReason() string {
	if i.ManuallyClosed() {
		return "工作项已关闭，请先重新打开后再触发 LOOP"
	}
	if i.TrackerClosed() || (i.Status == "closed" && i.CloseSource == "external") {
		return "外部工作项已关闭，请先在外部系统重新打开"
	}
	if i.DeliveryLocked() {
		return "工作项已交付闭环，请先重新打开后再触发 LOOP"
	}
	return ""
}

type IssueComment struct {
	ID         string    `json:"id"`
	IssueID    string    `json:"issueId"`
	AuthorType string    `json:"authorType"`
	AuthorID   string    `json:"authorId"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

type RejectRequest struct {
	Expected *NodeDecisionExpectation `json:"expected,omitempty"`
	NodeID   string                   `json:"nodeId"`
	TargetID string                   `json:"targetId"`
	Reason   string                   `json:"reason"`
	Context  string                   `json:"context,omitempty"`
}

// HumanDecision is the shared payload for approve / retry so operators can
// attach extra facts (failed test names, links) that later nodes will see.
type HumanDecision struct {
	Expected *NodeDecisionExpectation `json:"expected,omitempty"`
	NodeID   string                   `json:"nodeId"`
	Reason   string                   `json:"reason,omitempty"`
	Context  string                   `json:"context,omitempty"`
}

// NodeDecisionExpectation binds a workbench action to the attempt being viewed.
type NodeDecisionExpectation struct {
	Status      string `json:"status"`
	Attempt     int    `json:"attempt"`
	ReworkCount int    `json:"reworkCount"`
}
