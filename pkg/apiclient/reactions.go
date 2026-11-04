package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// Comment reactions live in Data Center's comment-likes plugin, and which
// endpoint applies depends on the server version (CapCommentReactions):
//
//	7.7 and later: PUT / DELETE .../comments/{id}/reactions/{emoticon}
//	before 7.7:    POST / DELETE .../comments/{id}/likes  (a like is thumbsup)
//
// Bitbucket Cloud has no reaction API, so the registry marks it unsupported.

// likeEmoticon is the reaction a pre-7.7 like stands for; 8.x still counts a
// thumbsup reaction as a like.
const likeEmoticon = "thumbsup"

// emoticonName matches a Data Center emoticon short name. Emoji characters are
// refused by the server with a bare 400, so they are rejected here with
// guidance instead.
var emoticonName = regexp.MustCompile(`^[A-Za-z0-9_+-]+$`)

// reactionCall is the resolved HTTP request for one reaction change.
type reactionCall struct {
	method, path, emoji string
}

// ReactPRComment adds or removes the caller's reaction on a PR comment.
func (c *apiClient) ReactPRComment(ctx context.Context, req ReactPRCommentReq) (*Reaction, error) {
	call, err := c.buildReactPRComment(ctx, req)
	if err != nil {
		return nil, err
	}
	out := &Reaction{CommentID: req.ID, PRID: req.PRID, Emoji: call.emoji, Removed: req.Remove}
	target := reactionWriteTarget(req.Repo, req.PRID, req.ID, call.emoji)
	if call.method != http.MethodPut {
		// DELETE, and the legacy like POST, answer 204 with no body.
		if err := c.doWriteJSON(ctx, call.method, call.path, nil, nil, nil, target); err != nil {
			return nil, withReactionHint(err, req, call)
		}
		return out, nil
	}
	var raw dcUserReaction
	if err := c.doWriteJSON(ctx, call.method, call.path, nil, nil, &raw, target); err != nil {
		return nil, withReactionHint(err, req, call)
	}
	if raw.Emoticon.Shortcut != "" {
		out.Emoji = raw.Emoticon.Shortcut
	}
	out.Value = raw.Emoticon.Value
	if raw.User != nil {
		u := mapDCUser(*raw.User)
		out.User = &u
	}
	return out, nil
}

// buildReactPRComment resolves the request for a reaction change. On Data
// Center it first reads the server version (a GET, safe under --dry-run),
// because the endpoint depends on it.
func (c *apiClient) buildReactPRComment(ctx context.Context, req ReactPRCommentReq) (reactionCall, error) {
	if err := checkRepoRef(req.Repo); err != nil {
		return reactionCall{}, err
	}
	emoji := strings.Trim(strings.TrimSpace(req.Emoji), ":")
	if !emoticonName.MatchString(emoji) {
		return reactionCall{}, cerrors.Newf(cerrors.CategoryUsage, "REACTION_BAD_EMOJI",
			"%q is not an emoticon short name", req.Emoji).
			WithHint("Pass the emoticon's short name, such as thumbsup, heart or laughing, not the emoji character.")
	}
	sup, version, err := c.versionedSupport(ctx, CapCommentReactions)
	if err != nil {
		return reactionCall{}, err
	}
	if !sup.Supported() {
		return reactionCall{}, cerrors.New(cerrors.CategoryUsage, "REACTION_UNSUPPORTED",
			"comment reactions are not available: "+sup.Reason).
			WithHint("Tell the user; ask before posting a text reply instead, since a reply is not a reaction.")
	}
	base := c.prCommentLikesPath(req.Repo, req.PRID, req.ID)
	if sup.Level == SupportEmulated {
		if emoji != likeEmoticon {
			ref := prName(req.Repo, req.PRID)
			return reactionCall{}, cerrors.Newf(cerrors.CategoryUsage, "REACTION_EMOJI_UNSUPPORTED",
				"Bitbucket Data Center %s accepts only the %s reaction (a like); other emoji need %s or later",
				version, likeEmoticon, c.supportFor(CapCommentReactions).Since).
				WithHint("Ask the user whether thumbsup will do; this server cannot show other emoji.").
				WithNextSteps(fmt.Sprintf("bitbucket-cli comment react %d --pr %s --emoji %s", req.ID, ref, likeEmoticon))
		}
		method := http.MethodPost
		if req.Remove {
			method = http.MethodDelete
		}
		return reactionCall{method: method, path: base + "/likes", emoji: emoji}, nil
	}
	method := http.MethodPut
	if req.Remove {
		method = http.MethodDelete
	}
	return reactionCall{method: method, path: base + "/reactions/" + url.PathEscape(emoji), emoji: emoji}, nil
}

// withReactionHint adds recovery guidance to a refused reaction without
// relabelling it: the server's own reason stays in the message. A 400 or 404
// usually means an unknown emoticon name or comment ID, removing a reaction
// the caller never added, or (before 7.7) liking one's own comment.
func withReactionHint(err error, req ReactPRCommentReq, call reactionCall) error {
	ce := cerrors.AsCLIError(err)
	if ce.HTTPStatus != http.StatusBadRequest && ce.HTTPStatus != http.StatusNotFound {
		return err
	}
	hint := "Check the comment ID and the emoticon's short name (thumbsup, heart, laughing). " +
		"If both are right, the server's Comment Likes or Emoticons system app may be disabled."
	if strings.HasSuffix(call.path, "/likes") {
		hint = "Check the comment ID. Servers before 7.7 do not let you like your own comment."
	}
	ref := prName(req.Repo, req.PRID)
	return ce.WithHint(hint).
		WithNextSteps(fmt.Sprintf("bitbucket-cli comment list --pr %s   # confirm the comment ID", ref))
}
