package domain

import (
	"strconv"
	"strings"
	"time"
)

// Issue is one requirement in the personal to-do list. It carries what a
// requirement is — a title, a description, a status, a priority, the project
// it belongs to, its tags and its links — and nothing that only made sense
// while the platform could run things.
type Issue struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`                // backlog, todo, in_progress, in_review, done, closed, blocked, cancelled
	CloseSource string `json:"closeSource,omitempty"` // manual, external; empty on older manually closed issues
	Priority    string `json:"priority"`
	// Tags are the requirement's own labels. The list filters by them.
	Tags []string `json:"tags,omitempty"`
	// RepositoryID is the legacy single-repository field (kept in sync with
	// the first entry of RepositoryIDs for display and external sync).
	RepositoryID string `json:"repositoryId,omitempty"`
	// RepositoryIDs lists every repository the requirement touches.
	RepositoryIDs []string `json:"repositoryIds,omitempty"`
	// BaseBranch is the legacy API key for an existing work branch to continue.
	// Empty creates a new work branch from the repository configured main branch.
	BaseBranch string `json:"baseBranch,omitempty"`
	// RepositoryBranches selects existing work branches by repository ID. When
	// provided, it replaces BaseBranch; an unlisted repository creates a new branch.
	RepositoryBranches map[string]string `json:"repositoryBranches,omitempty"`
	Kind               string            `json:"kind,omitempty"`   // requirement, issue
	Source             string            `json:"source,omitempty"` // local, github, gitlab
	ExternalRef        *ExternalRef      `json:"externalRef,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	// CompletedAt records an observed transition to done/closed, independent of
	// later edits. Older completed requirements may have no timestamp.
	CompletedAt time.Time `json:"completedAt,omitempty"`
	// Operations is the requirement's own audit trail: create, edit, close, reopen.
	Operations []IssueOperation `json:"operations,omitempty"`
}

// MaxIssueTags is how many labels one requirement carries.
const MaxIssueTags = 12

// MaxIssueTagLength is how long a single label may be.
const MaxIssueTagLength = 40

// NormalizeTags trims the labels, drops the empty and the duplicate ones and
// caps the list, so a requirement always carries a short, stable set.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		tag := strings.TrimSpace(raw)
		if tag == "" || seen[tag] {
			continue
		}
		if runes := []rune(tag); len(runes) > MaxIssueTagLength {
			tag = string(runes[:MaxIssueTagLength])
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) == MaxIssueTags {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// HasTag reports whether the requirement carries the label.
func (i Issue) HasTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, own := range i.Tags {
		if own == tag {
			return true
		}
	}
	return false
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
// repositories bound to the requirement. RepositoryIDs is authoritative;
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

// ExternalRef links a local requirement to an external issue in GitHub or GitLab.
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

const issueIdentityMarker = "需求身份（平台权威，禁止忽略）"

// IdentityBlock is the authoritative tracker identity of the requirement.
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
	Reason   string                   `json:"reason"`
	Context  string                   `json:"context,omitempty"`
}

// NodeDecisionExpectation binds a workbench action to the attempt being viewed.
type NodeDecisionExpectation struct {
	Status      string `json:"status"`
	Attempt     int    `json:"attempt"`
	ReworkCount int    `json:"reworkCount"`
}
