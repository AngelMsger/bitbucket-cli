package apiclient

import (
	"strconv"
	"strings"
	"time"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// Canonical review vote states surfaced by `pr inbox` (`my_vote`) and
// `pr status` (`my_review.vote`). Data Center reports "needs_work" while
// Bitbucket Cloud reports "changes_requested"; filter matching accepts both
// spellings on either flavor so automation stays portable.
const (
	VoteUnvoted          = "unvoted"
	VoteApproved         = "approved"
	VoteNeedsWork        = "needs_work"
	VoteChangesRequested = "changes_requested"
)

// AuthorKind values derived from platform account metadata.
const (
	AuthorKindHuman   = "human"
	AuthorKindBot     = "bot"
	AuthorKindUnknown = "unknown"
)

// NormalizeMyVoteFilter validates and normalizes a `--my-vote` filter value.
// An empty string means no vote filter.
func NormalizeMyVoteFilter(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.ReplaceAll(v, "-", "_")
	switch v {
	case "":
		return "", nil
	case VoteUnvoted, VoteApproved, VoteNeedsWork, VoteChangesRequested:
		return v, nil
	default:
		return "", cerrors.Newf(cerrors.CategoryUsage, "BAD_MY_VOTE",
			"invalid --my-vote value %q (expected unvoted, approved, needs_work, or changes_requested)", raw)
	}
}

// voteMatchesFilter reports whether a PR's resolved vote satisfies a normalized
// `--my-vote` filter. "needs_work" (DC) and "changes_requested" (Cloud) match
// interchangeably so cross-flavor queries work with either name.
func voteMatchesFilter(actual, want string) bool {
	if want == "" {
		return true
	}
	if actual == "" {
		actual = VoteUnvoted
	}
	if actual == want {
		return true
	}
	if isChangeRequestVote(actual) && isChangeRequestVote(want) {
		return true
	}
	return false
}

func isChangeRequestVote(v string) bool {
	return v == VoteNeedsWork || v == VoteChangesRequested
}

// userMatchesUser reports whether two User records refer to the same Bitbucket
// account by comparing stable identifiers (AccountID, UUID, Slug, Name). Empty
// identifiers never match.
func userMatchesUser(a, b User) bool {
	if idA, idB := normID(a.AccountID), normID(b.AccountID); idA != "" && idB != "" {
		return idA == idB
	}
	if idA, idB := normID(a.UUID), normID(b.UUID); idA != "" && idB != "" {
		return idA == idB
	}
	aKeys := nonEmptyNormIDs(a.AccountID, a.UUID, a.Slug, a.Name)
	bKeys := nonEmptyNormIDs(b.AccountID, b.UUID, b.Slug, b.Name)
	for _, ak := range aKeys {
		for _, bk := range bKeys {
			if ak == bk {
				return true
			}
		}
	}
	return false
}

func normID(v string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(v), "{}"))
}

func nonEmptyNormIDs(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if s := normID(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// participantVote derives the canonical vote state of a single Participant entry.
func participantVote(p Participant) string {
	state := strings.ToLower(strings.TrimSpace(p.State))
	switch state {
	case "approved":
		return VoteApproved
	case "needs_work":
		return VoteNeedsWork
	case "changes_requested":
		return VoteChangesRequested
	}
	if p.Approved {
		return VoteApproved
	}
	return VoteUnvoted
}

// resolveUserVote inspects a PR's participants and reviewers to determine the
// authenticated user's vote, Cloud participated_at timestamp (if present), and
// Data Center lastReviewedCommit hash (if present). Participants are checked
// alongside reviewers because Bitbucket Cloud stores vote state on participants
// while keeping reviewers as bare user stubs.
func resolveUserVote(pr *PullRequest, me *User) (vote, participatedAt, lastReviewedCommit string) {
	if pr == nil || me == nil {
		return "", "", ""
	}
	vote = VoteUnvoted
	inspect := func(group []Participant) {
		for _, p := range group {
			if !userMatchesUser(p.User, *me) {
				continue
			}
			pv := participantVote(p)
			if pv != VoteUnvoted {
				vote = pv
			}
			if p.ParticipatedAt != "" && participatedAt == "" {
				participatedAt = p.ParticipatedAt
			}
			if p.LastReviewedCommit != "" && lastReviewedCommit == "" {
				lastReviewedCommit = p.LastReviewedCommit
			}
		}
	}
	// Check Participants first (Cloud vote state lives here), then Reviewers
	// (Data Center reviewer vote state lives here).
	inspect(pr.Participants)
	inspect(pr.Reviewers)
	return vote, participatedAt, lastReviewedCommit
}

// ClassifyAuthorKind maps platform-reported account type metadata to "human",
// "bot", or "unknown". It never guesses from usernames or display names.
func ClassifyAuthorKind(u User) string {
	switch strings.ToLower(strings.TrimSpace(u.Type)) {
	case "user", "normal":
		return AuthorKindHuman
	case "app", "service", "bot":
		return AuthorKindBot
	default:
		return AuthorKindUnknown
	}
}

// IsAIAttributedText reports whether content begins with a standard leading AI
// attribution link marker such as `[[AI]](https://...)`. Matching only at the
// start of the trimmed comment prevents human replies that quote or mention
// `[[AI]](...)` inside blockquotes, backticks, or prose from being misclassified.
func IsAIAttributedText(content string) bool {
	s := strings.TrimSpace(content)
	const prefix = "[[ai]]("
	if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return false
	}
	after := s[len(prefix):]
	closeIdx := strings.IndexByte(after, ')')
	if closeIdx <= 0 {
		return false
	}
	target := after[:closeIdx]
	if strings.ContainsAny(target, " \t\r\n") {
		return false
	}
	return true
}

// EnrichCommentsWithSelf stamps the read-only IsSelf field on every comment in
// place when me is non-nil. When me is nil (identity unavailable), IsSelf is
// left nil so it is omitted from serialized JSON.
func EnrichCommentsWithSelf(comments []Comment, me *User) {
	if me == nil {
		return
	}
	for i := range comments {
		self := userMatchesUser(comments[i].Author, *me)
		comments[i].IsSelf = &self
	}
}

// EnrichThreadsWithSelf stamps IsSelf on every comment inside every thread.
func EnrichThreadsWithSelf(threads []Thread, me *User) {
	if me == nil {
		return
	}
	for i := range threads {
		EnrichCommentsWithSelf(threads[i].Comments, me)
	}
}

// parseBitbucketTime parses either a Cloud RFC 3339 timestamp or a Data Center
// millisecond-epoch string.
func parseBitbucketTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if ms, err := strconv.ParseInt(raw, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms).UTC(), true
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// formatRFC3339UTC normalizes a Bitbucket timestamp string (RFC 3339 or DC
// epoch millis) into a canonical UTC RFC 3339 string.
func formatRFC3339UTC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// commitsMatch compares an expected commit SHA (full or >=7-char prefix)
// against the PR's actual source commit hash.
func commitsMatch(actual, expected string) bool {
	a := strings.ToLower(strings.TrimSpace(actual))
	e := strings.ToLower(strings.TrimSpace(expected))
	if a == "" || e == "" {
		return false
	}
	if a == e {
		return true
	}
	if len(e) >= 7 && strings.HasPrefix(a, e) {
		return true
	}
	if len(a) >= 7 && strings.HasPrefix(e, a) {
		return true
	}
	return false
}
