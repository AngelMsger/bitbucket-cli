// Package apiclient is a flavor-agnostic Bitbucket REST client. It supports
// Bitbucket Cloud (REST 2.0) and Data Center / Server (REST 1.0) behind a
// single Client interface returning normalized models.
//
// This package backs the bitbucket-cli command layer and is also importable as a
// standalone client library (e.g. by a GUI); see the repository README. Its
// exported surface — the Client interface, the normalized models, and the
// read-only / dry-run semantics — is a contract the CLI and its companion
// Skill depend on. Extend it additively and keep existing shapes and behavior
// stable; do not reshape the public API to suit a single local call site.
package apiclient

// Flavor identifies the Bitbucket backend variant.
type Flavor string

const (
	FlavorCloud      Flavor = "cloud"
	FlavorDataCenter Flavor = "datacenter"
	FlavorAuto       Flavor = "auto"
)

// ServerInfo is the result of a connectivity probe.
type ServerInfo struct {
	Flavor    Flavor `json:"flavor"`
	BaseURL   string `json:"base_url"`
	Reachable bool   `json:"reachable"`
}

// User is a normalized Bitbucket user. Cloud identifies users by AccountID and
// nickname; Data Center by Name/Slug. Whichever the server returns is kept.
type User struct {
	AccountID   string `json:"account_id,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	Name        string `json:"name,omitempty"`
	Slug        string `json:"slug,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Type        string `json:"type,omitempty"`
}

// RepoRef identifies a repository inside its workspace (Cloud) or project (DC).
// Workspace holds the Cloud workspace slug or the DC project key.
type RepoRef struct {
	Workspace string `json:"workspace"`
	Slug      string `json:"slug"`
}

// Workspace is a normalized Bitbucket workspace (Cloud) / project (DC). The
// `Slug` field is the universal identifier `--workspace` flags accept across
// the CLI: a Cloud workspace slug or a DC project key.
type Workspace struct {
	Slug        string `json:"slug"`
	Name        string `json:"name,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	Type        string `json:"type,omitempty"` // PUBLIC | PRIVATE  (DC) or owner type (Cloud)
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// WorkspaceListOpts narrows a workspace listing.
type WorkspaceListOpts struct {
	ListOpts
	// Role (Cloud only): "owner" | "collaborator" | "member".
	Role string
	// Query is a server-side name substring filter.
	Query string
}

// UserListOpts narrows a user listing. Cloud's user listing is workspace-
// scoped (no global user endpoint), so Workspace is required there; Data
// Center has a global `/users` endpoint and Workspace is optional.
type UserListOpts struct {
	ListOpts
	Workspace string
	Query     string
}

// Tag is a normalized repository tag.
type Tag struct {
	Name    string `json:"name"`
	Target  string `json:"target,omitempty"` // commit hash
	Date    string `json:"date,omitempty"`
	Message string `json:"message,omitempty"`
}

// TagListOpts narrows a tag listing.
type TagListOpts struct {
	ListOpts
	Repo  RepoRef
	Query string
	Sort  string
}

// Repository is a normalized Bitbucket repository.
type Repository struct {
	UUID          string   `json:"uuid,omitempty"`
	Slug          string   `json:"slug"`
	Name          string   `json:"name,omitempty"`
	Workspace     string   `json:"workspace"`
	FullName      string   `json:"full_name,omitempty"`
	Description   string   `json:"description,omitempty"`
	Private       bool     `json:"private"`
	DefaultBranch string   `json:"default_branch,omitempty"`
	MainBranch    string   `json:"main_branch,omitempty"`
	Language      string   `json:"language,omitempty"`
	Size          int64    `json:"size,omitempty"`
	URL           string   `json:"url,omitempty"`
	CloneHTTPS    string   `json:"clone_https,omitempty"`
	CloneSSH      string   `json:"clone_ssh,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
	Links         []string `json:"links,omitempty"`
}

// BranchRef identifies a branch (and its target commit).
type BranchRef struct {
	Name   string `json:"name"`
	Target string `json:"target,omitempty"` // commit hash
}

// Branch is a normalized branch listing entry.
type Branch struct {
	Name        string `json:"name"`
	Target      string `json:"target,omitempty"`
	Default     bool   `json:"default,omitempty"`
	LastCommit  string `json:"last_commit,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// Commit is a normalized commit.
type Commit struct {
	Hash    string   `json:"hash"`
	Message string   `json:"message,omitempty"`
	Author  string   `json:"author,omitempty"`
	Date    string   `json:"date,omitempty"`
	Parents []string `json:"parents,omitempty"`
	URL     string   `json:"url,omitempty"`
}

// PRRef is the source / destination of a pull request.
type PRRef struct {
	Branch     string `json:"branch"`
	Commit     string `json:"commit,omitempty"`
	Repository string `json:"repository,omitempty"` // for cross-repo PRs (Cloud forks)
}

// Participant is a reviewer / participant on a PR.
type Participant struct {
	User     User   `json:"user"`
	Role     string `json:"role,omitempty"` // REVIEWER / PARTICIPANT
	Approved bool   `json:"approved,omitempty"`
	State    string `json:"state,omitempty"` // approved / changes_requested / null
}

// PullRequest is a normalized Bitbucket pull request.
type PullRequest struct {
	ID           int           `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description,omitempty"`
	State        string        `json:"state"` // OPEN / MERGED / DECLINED / SUPERSEDED
	Author       User          `json:"author"`
	Source       PRRef         `json:"source"`
	Destination  PRRef         `json:"destination"`
	Reviewers    []Participant `json:"reviewers,omitempty"`
	Participants []Participant `json:"participants,omitempty"`
	Repository   RepoRef       `json:"repository"`
	URL          string        `json:"url,omitempty"`
	CommentCount int           `json:"comment_count,omitempty"`
	TaskCount    int           `json:"task_count,omitempty"`
	CreatedAt    string        `json:"created_at,omitempty"`
	UpdatedAt    string        `json:"updated_at,omitempty"`
	ClosedAt     string        `json:"closed_at,omitempty"`
	MergeCommit  string        `json:"merge_commit,omitempty"`
}

// InlineAnchor pins an inline review comment to a path and line.
type InlineAnchor struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	From int    `json:"from,omitempty"` // origin line for the LHS (old / pre-change) side
	To   int    `json:"to,omitempty"`   // destination line for the RHS (new / post-change) side
	// LineType / FileType carry the diff classification resolved from the unified
	// diff (see ResolveInlineAnchor). They drive Data Center's anchor shape so an
	// added line is posted as ADDED/TO and a removed line as REMOVED/FROM, rather
	// than the old always-CONTEXT/TO guess. Empty on comments read back from the
	// API unless the server supplied them.
	LineType string `json:"line_type,omitempty"` // ADDED | REMOVED | CONTEXT
	FileType string `json:"file_type,omitempty"` // TO | FROM
}

// Comment is a normalized PR or commit comment.
type Comment struct {
	ID        int           `json:"id"`
	Content   string        `json:"content"`
	Author    User          `json:"author"`
	Inline    *InlineAnchor `json:"inline,omitempty"`
	ParentID  int           `json:"parent_id,omitempty"`
	PRID      int           `json:"pr_id,omitempty"`
	CommitID  string        `json:"commit_id,omitempty"`
	URL       string        `json:"url,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	UpdatedAt string        `json:"updated_at,omitempty"`
	// Resolved is true when the comment's thread has been marked resolved.
	// Cloud: derived from the `resolution` object; DC: state == "RESOLVED".
	// Settable with ResolvePRComment (`comment resolve` / `--unresolve`).
	Resolved bool `json:"resolved"`
	// Task is true for actionable review tasks. DC: severity == "BLOCKER", and
	// completing such a task is the same transition as ResolvePRComment. (Cloud
	// tasks live on a separate endpoint and are not surfaced yet.)
	Task bool `json:"task,omitempty"`
}

// Activity is one entry in a PR's activity stream.
type Activity struct {
	Kind     string   `json:"kind"` // comment / approval / update / merge / decline
	Actor    User     `json:"actor"`
	When     string   `json:"when,omitempty"`
	Comment  *Comment `json:"comment,omitempty"`
	Approved bool     `json:"approved,omitempty"`
	State    string   `json:"state,omitempty"`
}

// ListResult is one page of a paginated listing. Next is an opaque cursor for
// the following page, empty when the listing is exhausted.
type ListResult[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next,omitempty"`
}

// ListOpts controls a paginated listing.
type ListOpts struct {
	Limit  int
	Cursor string
}

// RepoListOpts narrows a repository listing.
type RepoListOpts struct {
	ListOpts
	Workspace string // Cloud workspace slug / DC project key
	Role      string // owner / contributor / member (Cloud only)
	Query     string // server-side query (Cloud "q=")
	Sort      string // Cloud sort key
}

// PRListOpts narrows a PR listing.
type PRListOpts struct {
	ListOpts
	Repo     RepoRef
	State    string // OPEN (default) / MERGED / DECLINED / ALL
	Author   string
	Reviewer string
	Source   string // source branch
	Target   string // destination branch
	Query    string
}

// PRScope controls how much detail to fetch when getting a single PR.
type PRScope string

const (
	PRScopeSummary  PRScope = "summary"
	PRScopeFull     PRScope = "full"
	PRScopeDiff     PRScope = "diff"
	PRScopeCommits  PRScope = "commits"
	PRScopeActivity PRScope = "activity"
)

// GetPROpts controls a PR fetch.
type GetPROpts struct {
	Repo  RepoRef
	ID    int
	Scope PRScope
}

// CreatePRReq is a request to open a new PR.
type CreatePRReq struct {
	Repo              RepoRef
	Title             string
	Description       string
	Source            string // source branch
	SourceRepo        string // optional, defaults to Repo for non-fork PRs
	Destination       string // target branch; empty -> repo default branch
	Reviewers         []string
	CloseSourceBranch bool
}

// UpdatePRReq edits an existing PR's title / description / reviewers.
type UpdatePRReq struct {
	Repo        RepoRef
	ID          int
	Title       string
	Description string
	Reviewers   []string // when non-nil, replaces the reviewer list
}

// DeclinePRReq closes an open PR without merging.
type DeclinePRReq struct {
	Repo    RepoRef
	ID      int
	Message string
}

// MergePRReq merges a PR. Strategy values:
//   - merge_commit: --no-ff merge commit (Cloud "merge_commit", DC "merge-commit")
//   - squash:       single squashed commit
//   - fast_forward: fast-forward when possible (Cloud "fast_forward", DC "ff")
type MergePRReq struct {
	Repo              RepoRef
	ID                int
	Strategy          string
	Message           string
	CloseSourceBranch bool
}

// ApprovePRReq toggles an approval on a PR.
type ApprovePRReq struct {
	Repo    RepoRef
	ID      int
	Approve bool // false = withdraw approval
}

// RequestChangesReq toggles a "request changes" vote (Cloud-only).
type RequestChangesReq struct {
	Repo    RepoRef
	ID      int
	Request bool // false = withdraw
}

// ListPRCommentsOpts narrows a PR comment listing.
type ListPRCommentsOpts struct {
	ListOpts
	Repo RepoRef
	PRID int
}

// AddPRCommentReq creates a comment (general or inline) on a PR.
type AddPRCommentReq struct {
	Repo    RepoRef
	PRID    int
	Content string
	Inline  *InlineAnchor // nil = general comment
	ReplyTo int           // 0 = top-level comment
}

// UpdatePRCommentReq edits a comment's content.
type UpdatePRCommentReq struct {
	Repo    RepoRef
	PRID    int
	ID      int
	Content string
}

// DeletePRCommentReq removes a comment.
type DeletePRCommentReq struct {
	Repo RepoRef
	PRID int
	ID   int
}

// ResolvePRCommentReq toggles a comment thread's resolution state. Resolve=true
// marks the thread resolved; false reopens it. On Data Center this is the same
// transition that completes/reopens a task comment (severity == BLOCKER).
type ResolvePRCommentReq struct {
	Repo    RepoRef
	PRID    int
	ID      int
	Resolve bool
}

// ListCommitsOpts narrows a commit listing.
type ListCommitsOpts struct {
	ListOpts
	Repo   RepoRef
	Branch string
	Path   string
	Since  string
	Until  string
}

// CompareCommitsReq compares two refs (branches or hashes).
type CompareCommitsReq struct {
	Repo RepoRef
	From string
	To   string
}

// BranchListOpts narrows a branch listing.
type BranchListOpts struct {
	ListOpts
	Repo  RepoRef
	Query string
	Sort  string
}

// CreateBranchReq creates a branch from a ref.
type CreateBranchReq struct {
	Repo    RepoRef
	Name    string
	FromRef string
}

// DeleteBranchReq removes a branch.
type DeleteBranchReq struct {
	Repo RepoRef
	Name string
}

// CreateRepoReq creates a repository.
type CreateRepoReq struct {
	Workspace   string
	Slug        string
	Name        string
	Description string
	Private     bool
}

// DeleteRepoReq removes a repository.
type DeleteRepoReq struct {
	Repo RepoRef
}

// WriteRequestPlan describes the HTTP request a write operation would send,
// without sending it. It is used to render --dry-run previews.
type WriteRequestPlan struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Payload any    `json:"payload,omitempty"`
}

// --- v0.2 file browsing + PR review aggregation models ---

// FileEntry is one entry from a repository file listing.
type FileEntry struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Type   string `json:"type"` // "file" | "dir" | "link" | "submodule"
	Size   int64  `json:"size,omitempty"`
	Hash   string `json:"hash,omitempty"`   // git blob hash if exposed
	Commit string `json:"commit,omitempty"` // commit that last touched this entry (Cloud only)
}

// FileContent is the raw byte content of a file at a given ref.
type FileContent struct {
	Path      string `json:"path"`
	Ref       string `json:"ref,omitempty"`
	Bytes     []byte `json:"-"` // raw bytes; not JSON-marshalled (callers stream directly)
	Size      int64  `json:"size"`
	Encoding  string `json:"encoding,omitempty"` // "utf-8" | "binary"
	Truncated bool   `json:"truncated,omitempty"`
}

// Diffstat is one row of a PR's per-file change summary.
type Diffstat struct {
	Path         string `json:"path"`
	OldPath      string `json:"old_path,omitempty"`
	Status       string `json:"status"` // "added" | "modified" | "removed" | "renamed" | "copied"
	LinesAdded   int    `json:"added"`
	LinesRemoved int    `json:"removed"`
	Binary       bool   `json:"binary,omitempty"`
}

// Thread is an inline review thread, grouped by file (and anchor).
// File is "" for general (non-inline) discussions.
type Thread struct {
	File     string        `json:"file,omitempty"`
	Anchor   *InlineAnchor `json:"anchor,omitempty"`
	Comments []Comment     `json:"comments"`
	// Resolved mirrors the root comment's Resolved status for the thread.
	Resolved bool `json:"resolved"`
}

// MergeCheck is the server-side pre-merge verdict for a PR.
type MergeCheck struct {
	CanMerge   bool     `json:"can_merge"`
	Conflicted bool     `json:"conflicted"`
	Outcome    string   `json:"outcome,omitempty"`
	Vetoes     []string `json:"vetoes,omitempty"`
}

// BuildStatus is one CI / build report attached to a commit.
type BuildStatus struct {
	Key         string `json:"key"`
	Name        string `json:"name,omitempty"`
	State       string `json:"state"` // SUCCESSFUL | INPROGRESS | FAILED | STOPPED
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	CommitHash  string `json:"commit_hash,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// PRStatus is the aggregated "is this PR ready to merge?" view returned by
// the `pr status` command. Each field is independently fetched and may be nil
// if the server omits / errors on that piece.
type PRStatus struct {
	PR         *PullRequest  `json:"pr"`
	MergeCheck *MergeCheck   `json:"merge_check,omitempty"`
	Reviewers  []Participant `json:"reviewers,omitempty"`
	Builds     []BuildStatus `json:"builds,omitempty"`
}

// FileListOpts narrows a directory listing.
type FileListOpts struct {
	ListOpts
	Repo RepoRef
	Ref  string // branch / tag / commit hash; empty = repo default branch
	Path string // "" = repo root
}

// FileGetOpts controls a single file fetch.
type FileGetOpts struct {
	Repo RepoRef
	Ref  string
	Path string
}

// TreeOpts controls a recursive tree walk.
type TreeOpts struct {
	Repo  RepoRef
	Ref   string
	Path  string
	Depth int // 0 = unlimited
}

// MyPRListOpts narrows a cross-repo "PRs involving me" listing.
//
// Role values: "REVIEWER" (default) | "AUTHOR" | "PARTICIPANT".
// State values: "OPEN" (default) | "MERGED" | "DECLINED" | "ALL".
//
// Workspace is optional on Data Center (the dashboard endpoint searches every
// project the user can see) and required on Cloud when Role == "REVIEWER" —
// Cloud has no global reviewer index, so the CLI fans out across the repos
// in the named workspace.
type MyPRListOpts struct {
	ListOpts
	Role      string
	State     string
	Workspace string
}
