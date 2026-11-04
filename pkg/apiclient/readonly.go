package apiclient

import (
	"context"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// NewReadOnly wraps inner so that every mutating method returns a
// READONLY_BLOCKED error before any HTTP request is sent. Reads (the Reader
// half of Client, including DescribeWrite, the --dry-run preview path) pass
// straight through, so safe inspection still works.
func NewReadOnly(inner Client) Client { return &readOnlyClient{Reader: inner} }

// readOnlyClient is the read-only enforcement layer. It keeps inner only as a
// Reader, so reads are delegated unchanged and no write can reach inner. Every
// Writer method is overridden below to return READONLY_BLOCKED; the assertion
// that follows stops compilation when a new Writer method lacks its override.
type readOnlyClient struct{ Reader }

var _ Client = (*readOnlyClient)(nil)

// blocked returns the family's READONLY_BLOCKED error for a refused write. op
// names the operation (e.g. "CreatePR") so the message says which call was
// refused. Turning read-only mode off is the user's decision, so the error
// names the user's switches without offering to change them.
func blocked(op string) *cerrors.CLIError {
	return cerrors.ReadOnlyBlocked(op, cerrors.ReadOnlyMode{
		Env:        "BITBUCKET_CLI_READ_ONLY",
		ConfigFile: "~/.angelmsger/bitbucket/config.yaml",
		DryRun:     true,
	})
}

// Repository writes.
func (r *readOnlyClient) CreateRepository(_ context.Context, _ CreateRepoReq) (*Repository, error) {
	return nil, blocked("CreateRepository")
}
func (r *readOnlyClient) ForkRepository(_ context.Context, _ ForkRepoReq) (*Repository, error) {
	return nil, blocked("ForkRepository")
}
func (r *readOnlyClient) DeleteRepository(_ context.Context, _ DeleteRepoReq) error {
	return blocked("DeleteRepository")
}

// PR writes.
func (r *readOnlyClient) CreatePR(_ context.Context, _ CreatePRReq) (*PullRequest, error) {
	return nil, blocked("CreatePR")
}
func (r *readOnlyClient) UpdatePR(_ context.Context, _ UpdatePRReq) (*PullRequest, error) {
	return nil, blocked("UpdatePR")
}
func (r *readOnlyClient) DeclinePR(_ context.Context, _ DeclinePRReq) (*PullRequest, error) {
	return nil, blocked("DeclinePR")
}
func (r *readOnlyClient) MergePR(_ context.Context, _ MergePRReq) (*PullRequest, error) {
	return nil, blocked("MergePR")
}
func (r *readOnlyClient) ApprovePR(_ context.Context, _ ApprovePRReq) error {
	return blocked("ApprovePR")
}
func (r *readOnlyClient) RequestPRChanges(_ context.Context, _ RequestChangesReq) error {
	return blocked("RequestPRChanges")
}

// Comment writes.
func (r *readOnlyClient) AddPRComment(_ context.Context, _ AddPRCommentReq) (*Comment, error) {
	return nil, blocked("AddPRComment")
}
func (r *readOnlyClient) UpdatePRComment(_ context.Context, _ UpdatePRCommentReq) (*Comment, error) {
	return nil, blocked("UpdatePRComment")
}
func (r *readOnlyClient) DeletePRComment(_ context.Context, _ DeletePRCommentReq) error {
	return blocked("DeletePRComment")
}
func (r *readOnlyClient) ResolvePRComment(_ context.Context, _ ResolvePRCommentReq) (*Comment, error) {
	return nil, blocked("ResolvePRComment")
}
func (r *readOnlyClient) ReactPRComment(_ context.Context, _ ReactPRCommentReq) (*Reaction, error) {
	return nil, blocked("ReactPRComment")
}

// Branch writes.
func (r *readOnlyClient) CreateBranch(_ context.Context, _ CreateBranchReq) (*Branch, error) {
	return nil, blocked("CreateBranch")
}
func (r *readOnlyClient) DeleteBranch(_ context.Context, _ DeleteBranchReq) error {
	return blocked("DeleteBranch")
}
