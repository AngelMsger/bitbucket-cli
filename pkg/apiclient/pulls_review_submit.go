package apiclient

import (
	"context"
	"fmt"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// ReviewVerdict is the top-level verdict of a composite `pr review` submission.
type ReviewVerdict string

const (
	ReviewVerdictApprove        ReviewVerdict = "approve"
	ReviewVerdictRequestChanges ReviewVerdict = "request-changes"
	ReviewVerdictCommentOnly    ReviewVerdict = "comment-only"
)

// NormalizeReviewVerdict validates and normalizes a review verdict flag/input.
func NormalizeReviewVerdict(raw string) (ReviewVerdict, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "_", "-")
	switch ReviewVerdict(v) {
	case ReviewVerdictApprove, ReviewVerdictRequestChanges, ReviewVerdictCommentOnly:
		return ReviewVerdict(v), nil
	default:
		return "", cerrors.Newf(cerrors.CategoryUsage, "BAD_REVIEW_VERDICT",
			"invalid review verdict %q (expected approve, request-changes, or comment-only)", raw)
	}
}

// ReviewCommentInput describes one comment to publish as part of a composite
// `pr review` submission. A comment is one of:
//   - general: Path == "", Line == 0, ReplyTo == 0
//   - inline:  Path != "" and Line > 0 (Side defaults to "new")
//   - reply:   ReplyTo > 0
type ReviewCommentInput struct {
	Content string `json:"content" yaml:"content"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Line    int    `json:"line,omitempty" yaml:"line,omitempty"`
	Side    string `json:"side,omitempty" yaml:"side,omitempty"`
	ReplyTo int    `json:"reply_to,omitempty" yaml:"reply_to,omitempty"`
}

// SubmitPRReviewReq submits zero or more comments and an optional verdict in
// two phases: validate everything up front (expected source commit + all inline
// diff anchors + vote plan), then execute comments sequentially and apply the
// verdict only if every comment succeeds.
type SubmitPRReviewReq struct {
	Repo                 RepoRef
	PRID                 int
	Verdict              ReviewVerdict
	ExpectedSourceCommit string
	Comments             []ReviewCommentInput
}

// PRReviewCommentPlan is the validated plan for one comment in a composite review.
type PRReviewCommentPlan struct {
	Index   int              `json:"index"`
	Kind    string           `json:"kind"` // general | inline | reply
	Inline  *InlineAnchor    `json:"inline,omitempty"`
	ReplyTo int              `json:"reply_to,omitempty"`
	Request WriteRequestPlan `json:"request"`
}

// PRReviewPlan is the full validated dry-run preview of a composite review.
type PRReviewPlan struct {
	PR           string                `json:"pr"`
	SourceCommit string                `json:"source_commit,omitempty"`
	Verdict      ReviewVerdict         `json:"verdict"`
	Comments     []PRReviewCommentPlan `json:"comments"`
	Vote         *WriteRequestPlan     `json:"vote,omitempty"`
	Validated    bool                  `json:"validated"`
}

// PRReviewFailure records the exact step where Phase 2 execution stopped.
type PRReviewFailure struct {
	Kind    string               `json:"kind"` // comment | vote
	Index   int                  `json:"index,omitempty"`
	Comment *ReviewCommentInput  `json:"comment,omitempty"`
	Verdict ReviewVerdict        `json:"verdict,omitempty"`
	Error   *cerrors.PayloadBody `json:"error"`
}

// PRReviewResult is the execution outcome of SubmitPRReview. When a mid-flight
// write fails, SubmitPRReview returns both the partial PRReviewResult (so the
// caller can see every posted comment ID and whether the vote ran) and a
// non-retryable structured error.
type PRReviewResult struct {
	PR             string           `json:"pr"`
	SourceCommit   string           `json:"source_commit,omitempty"`
	Verdict        ReviewVerdict    `json:"verdict"`
	PostedComments []Comment        `json:"posted_comments"`
	FailedItem     *PRReviewFailure `json:"failed_item,omitempty"`
	VoteApplied    bool             `json:"vote_applied"`
}

type preparedPRReview struct {
	plan        *PRReviewPlan
	commentReqs []AddPRCommentReq
}

// PlanPRReview runs Phase 1 validation and builds every planned HTTP request
// without mutating Bitbucket. Usable in read-only mode and for `--dry-run`.
func (c *apiClient) PlanPRReview(ctx context.Context, req SubmitPRReviewReq) (*PRReviewPlan, error) {
	prep, err := c.preparePRReview(ctx, req)
	if err != nil {
		return nil, err
	}
	return prep.plan, nil
}

// SubmitPRReview validates the entire review up front (zero remote writes on
// any validation error), then posts comments sequentially and casts the verdict
// only after every comment succeeds.
func (c *apiClient) SubmitPRReview(ctx context.Context, req SubmitPRReviewReq) (*PRReviewResult, error) {
	prep, err := c.preparePRReview(ctx, req)
	if err != nil {
		return nil, err
	}

	res := &PRReviewResult{
		PR:             prep.plan.PR,
		SourceCommit:   prep.plan.SourceCommit,
		Verdict:        prep.plan.Verdict,
		PostedComments: []Comment{},
	}

	for i, creq := range prep.commentReqs {
		cm, cerr := c.AddPRComment(ctx, creq)
		if cerr != nil {
			ce := cerrors.AsCLIError(cerr)
			body := ce.Payload().Error
			itemCopy := req.Comments[i]
			res.FailedItem = &PRReviewFailure{
				Kind:    "comment",
				Index:   i,
				Comment: &itemCopy,
				Error:   &body,
			}
			return res, reviewPartialFailure(ce.Category, res)
		}
		res.PostedComments = append(res.PostedComments, *cm)
	}

	switch prep.plan.Verdict {
	case ReviewVerdictApprove:
		if verr := c.ApprovePR(ctx, ApprovePRReq{Repo: req.Repo, ID: req.PRID, Approve: true}); verr != nil {
			ce := cerrors.AsCLIError(verr)
			body := ce.Payload().Error
			res.FailedItem = &PRReviewFailure{
				Kind:    "vote",
				Verdict: prep.plan.Verdict,
				Error:   &body,
			}
			return res, reviewPartialFailure(ce.Category, res)
		}
		res.VoteApplied = true
	case ReviewVerdictRequestChanges:
		if verr := c.RequestPRChanges(ctx, RequestChangesReq{Repo: req.Repo, ID: req.PRID, Request: true}); verr != nil {
			ce := cerrors.AsCLIError(verr)
			body := ce.Payload().Error
			res.FailedItem = &PRReviewFailure{
				Kind:    "vote",
				Verdict: prep.plan.Verdict,
				Error:   &body,
			}
			return res, reviewPartialFailure(ce.Category, res)
		}
		res.VoteApplied = true
	case ReviewVerdictCommentOnly:
		// No vote write.
	}

	return res, nil
}

func (c *apiClient) preparePRReview(ctx context.Context, req SubmitPRReviewReq) (*preparedPRReview, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return nil, err
	}
	if req.PRID <= 0 {
		return nil, cerrors.New(cerrors.CategoryUsage, "PR_NO_ID", "a positive PR ID is required")
	}
	verdict, err := NormalizeReviewVerdict(string(req.Verdict))
	if err != nil {
		return nil, err
	}
	if verdict == ReviewVerdictCommentOnly && len(req.Comments) == 0 {
		return nil, cerrors.New(cerrors.CategoryUsage, "REVIEW_EMPTY",
			"a comment-only review requires at least one comment").
			WithHint("Provide comments via --input, or pass --verdict approve / --verdict request-changes.")
	}

	needsDiff := false
	for i, item := range req.Comments {
		if strings.TrimSpace(item.Content) == "" {
			return nil, cerrors.Newf(cerrors.CategoryUsage, "COMMENT_NO_BODY",
				"review comment[%d]: content must not be empty", i)
		}
		if item.ReplyTo < 0 {
			return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_REVIEW_COMMENT",
				"review comment[%d]: reply_to must be positive", i)
		}
		hasInline := item.Path != "" || item.Line != 0 || item.Side != ""
		if item.ReplyTo > 0 && hasInline {
			return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_REVIEW_COMMENT",
				"review comment[%d]: cannot combine reply_to with inline path/line/side", i)
		}
		if hasInline {
			if strings.TrimSpace(item.Path) == "" || item.Line <= 0 {
				return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_INLINE",
					"review comment[%d]: inline comment requires both path and a positive line number", i)
			}
			if item.Side != "" && item.Side != DiffSideNew && item.Side != DiffSideOld {
				return nil, cerrors.Newf(cerrors.CategoryUsage, "BAD_SIDE",
					"review comment[%d]: side must be %q or %q, got %q", i, DiffSideNew, DiffSideOld, item.Side)
			}
			needsDiff = true
		}
	}

	prRefStr := normalizedPRRef(req.Repo, req.PRID)
	pr, err := c.GetPR(ctx, GetPROpts{Repo: req.Repo, ID: req.PRID, Scope: PRScopeSummary})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.ExpectedSourceCommit) != "" && !commitsMatch(pr.Source.Commit, req.ExpectedSourceCommit) {
		return nil, cerrors.Newf(cerrors.CategoryConflict, "PR_HEAD_MOVED",
			"PR %s source commit is now %q, expected %q; aborting review without writing",
			prRefStr, pr.Source.Commit, req.ExpectedSourceCommit).
			WithHint("New commits were pushed to the PR since you inspected it. Refresh the PR and diff before submitting your review.").
			WithNextSteps(
				"bitbucket-cli pr status "+prRefStr,
				"bitbucket-cli pr diff "+prRefStr,
			)
	}

	var diffFiles []FileDiff
	perPathCache := map[string]*FileDiff{}
	if needsDiff {
		diffFiles, err = c.GetPRFileDiffs(ctx, req.Repo, req.PRID, "")
		if err != nil {
			return nil, err
		}
	}

	plan := &PRReviewPlan{
		PR:           prRefStr,
		SourceCommit: pr.Source.Commit,
		Verdict:      verdict,
		Comments:     make([]PRReviewCommentPlan, 0, len(req.Comments)),
		Validated:    true,
	}
	commentReqs := make([]AddPRCommentReq, 0, len(req.Comments))

	for i, item := range req.Comments {
		var (
			anchor *InlineAnchor
			kind   = "general"
		)
		if item.ReplyTo > 0 {
			kind = "reply"
		} else if item.Path != "" {
			kind = "inline"
			fd := FindFileDiff(diffFiles, item.Path)
			if fd == nil {
				if cached, ok := perPathCache[item.Path]; ok {
					fd = cached
				} else {
					pfiles, perr := c.GetPRFileDiffs(ctx, req.Repo, req.PRID, item.Path)
					if perr == nil {
						fd = FindFileDiff(pfiles, item.Path)
					}
					perPathCache[item.Path] = fd
				}
			}
			if fd == nil {
				return nil, cerrors.Newf(cerrors.CategoryUsage, "INLINE_FILE_NOT_IN_DIFF",
					"review comment[%d]: file %s is not part of this PR's diff", i, item.Path).
					WithHint("Check the path against `pr files`, or omit path/line to post a general comment.")
			}
			anchor, err = ResolveInlineAnchor(fd, item.Line, item.Side)
			if err != nil {
				ce := cerrors.AsCLIError(err)
				wrapped := cerrors.Newf(ce.Category, ce.Code, "review comment[%d]: %s", i, ce.Message)
				wrapped.Hint = ce.Hint
				wrapped.NextSteps = ce.NextSteps
				return nil, wrapped
			}
		}
		creq := AddPRCommentReq{
			Repo:    req.Repo,
			PRID:    req.PRID,
			Content: item.Content,
			Inline:  anchor,
			ReplyTo: item.ReplyTo,
		}
		m, p, payload := c.buildAddPRComment(creq)
		commentReqs = append(commentReqs, creq)
		plan.Comments = append(plan.Comments, PRReviewCommentPlan{
			Index:   i,
			Kind:    kind,
			Inline:  anchor,
			ReplyTo: item.ReplyTo,
			Request: WriteRequestPlan{Method: m, URL: c.baseURL + p, Payload: payload},
		})
	}

	switch verdict {
	case ReviewVerdictApprove:
		plan.Vote = &WriteRequestPlan{
			Method: "POST",
			URL:    c.baseURL + c.prPath(req.Repo, req.PRID) + "/approve",
		}
	case ReviewVerdictRequestChanges:
		m, p, payload, verr := c.buildRequestPRChanges(ctx, RequestChangesReq{
			Repo: req.Repo, ID: req.PRID, Request: true,
		})
		if verr != nil {
			return nil, verr
		}
		plan.Vote = &WriteRequestPlan{
			Method:  m,
			URL:     c.baseURL + p,
			Payload: payload,
		}
	}

	return &preparedPRReview{plan: plan, commentReqs: commentReqs}, nil
}

func reviewPartialFailure(cat cerrors.Category, res *PRReviewResult) error {
	var msg string
	if res.FailedItem != nil && res.FailedItem.Kind == "vote" {
		msg = fmt.Sprintf("posted %d comment(s), but submitting verdict %q failed; see partial result on stdout",
			len(res.PostedComments), res.FailedItem.Verdict)
	} else {
		idx := 0
		if res.FailedItem != nil {
			idx = res.FailedItem.Index
		}
		msg = fmt.Sprintf("review stopped at comment[%d] after posting %d comment(s); vote was not applied; see partial result on stdout",
			idx, len(res.PostedComments))
	}
	e := cerrors.New(cat, "REVIEW_PARTIAL_FAILURE", msg).
		WithHint("Do not rerun the full review blindly: comments listed in posted_comments were already published. Reconcile the PR state and submit only the remaining items.").
		WithNextSteps(
			"bitbucket-cli pr threads "+res.PR,
			"bitbucket-cli pr status "+res.PR,
		)
	e.Retryable = false
	return e
}
