package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestParseServerVersion(t *testing.T) {
	floor := parseServerVersion("7.7")
	cases := []struct {
		raw          string
		known, atMin bool
	}{
		{"8.19.3", true, true},
		{"7.7.0", true, true},
		{"7.7", true, true},
		{"10.0.1", true, true},
		{"9.4.0-rc1", true, true},
		{"7.6.12", true, false},
		{"7.6", true, false},
		{"7", true, false},
		{"6.10.17", true, false},
		{"", false, false},
		{"unknown", false, false},
	}
	for _, tc := range cases {
		v := parseServerVersion(tc.raw)
		if v.known != tc.known {
			t.Errorf("parseServerVersion(%q).known = %t, want %t", tc.raw, v.known, tc.known)
		}
		if tc.known && v.atLeast(floor) != tc.atMin {
			t.Errorf("%q atLeast 7.7 = %t, want %t", tc.raw, v.atLeast(floor), tc.atMin)
		}
	}
}

func TestSupportAtVersion(t *testing.T) {
	row := Support{Level: SupportNative, Since: "7.7",
		Before: &Support{Level: SupportEmulated, Reason: "like only"}}
	if got := row.at(parseServerVersion("8.0.0")).Level; got != SupportNative {
		t.Errorf("8.0.0 level = %s, want native", got)
	}
	if got := row.at(parseServerVersion("7.6.3")).Level; got != SupportEmulated {
		t.Errorf("7.6.3 level = %s, want emulated", got)
	}
	if got := row.at(parseServerVersion("")).Level; got != SupportNative {
		t.Errorf("unknown version level = %s, want native (treated as current)", got)
	}
	noFallback := Support{Level: SupportNative, Since: "8.9"}
	if got := noFallback.at(parseServerVersion("8.5.0")); got.Supported() || got.Reason == "" {
		t.Errorf("below Since without Before = %+v, want unsupported with a reason", got)
	}
}

// reactionServer answers application-properties with version and records
// every other request. It replies to PUT with a RestUserReaction body and to
// everything else with 204, like the comment-likes plugin.
func reactionServer(t *testing.T, version string, writes *[]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/rest/api/1.0/application-properties" {
			_, _ = fmt.Fprintf(w, `{"version":%q}`, version)
			return
		}
		if r.Method == http.MethodGet {
			t.Errorf("unexpected read: %s", r.URL.RequestURI())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		*writes = append(*writes, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPut {
			_, _ = w.Write([]byte(`{"emoticon":{"shortcut":"heart","value":"❤️","url":"https://bb.example/heart.png"},` +
				`"user":{"name":"alice","slug":"alice","displayName":"Alice"}}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// Each server version must preview the request it executes, and the preview
// must send no write, including under the read-only wrapper.
func TestCommentReactionsMatchRegistry(t *testing.T) {
	const base = "/rest/comment-likes/latest/projects/PROJ/repos/repo/pull-requests/7/comments/42"
	cases := []struct {
		version, emoji string
		remove         bool
		want           string
	}{
		{"8.19.0", "heart", false, "PUT " + base + "/reactions/heart"},
		{"8.19.0", ":heart:", true, "DELETE " + base + "/reactions/heart"},
		{"7.7.0", "thumbsup", false, "PUT " + base + "/reactions/thumbsup"},
		{"", "heart", false, "PUT " + base + "/reactions/heart"},
		{"7.6.3", "thumbsup", false, "POST " + base + "/likes"},
		{"7.6.3", "thumbsup", true, "DELETE " + base + "/likes"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s/remove=%t", tc.version, tc.emoji, tc.remove), func(t *testing.T) {
			var writes []string
			c := newWriteTestClient(t, FlavorDataCenter, reactionServer(t, tc.version, &writes))
			req := ReactPRCommentReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42,
				Emoji: tc.emoji, Remove: tc.remove}
			plan, err := NewReadOnly(c).DescribeWrite(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if len(writes) != 0 {
				t.Fatalf("preview sent writes: %v", writes)
			}
			if got := plan.Method + " " + strings.TrimPrefix(plan.URL, c.BaseURL()); got != tc.want {
				t.Fatalf("preview = %s, want %s", got, tc.want)
			}
			if plan.Payload != nil {
				t.Fatalf("preview payload = %#v, want none", plan.Payload)
			}
			r, err := c.ReactPRComment(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if len(writes) != 1 || writes[0] != tc.want {
				t.Fatalf("live writes = %v, want [%s]", writes, tc.want)
			}
			if r.CommentID != 42 || r.PRID != 7 || r.Removed != tc.remove {
				t.Fatalf("result = %+v", r)
			}
		})
	}
}

func TestCommentReactionAddMapsServerReaction(t *testing.T) {
	var writes []string
	c := newWriteTestClient(t, FlavorDataCenter, reactionServer(t, "8.19.0", &writes))
	r, err := c.ReactPRComment(context.Background(), ReactPRCommentReq{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42, Emoji: "heart"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Emoji != "heart" || r.Value != "❤️" || r.User == nil || r.User.Name != "alice" {
		t.Fatalf("reaction = %+v (user %+v)", r, r.User)
	}
}

// Before 7.7 only a like exists: other emoji fail before any write and say
// which version would accept them.
func TestCommentReactionOldServerRejectsOtherEmoji(t *testing.T) {
	var writes []string
	c := newWriteTestClient(t, FlavorDataCenter, reactionServer(t, "7.6.3", &writes))
	req := ReactPRCommentReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42, Emoji: "heart"}
	for name, call := range map[string]func() error{
		"preview": func() error { _, err := c.DescribeWrite(context.Background(), req); return err },
		"live":    func() error { _, err := c.ReactPRComment(context.Background(), req); return err },
	} {
		ce := cerrors.AsCLIError(call())
		if ce == nil || ce.Code != "REACTION_EMOJI_UNSUPPORTED" || ce.Category != cerrors.CategoryUsage {
			t.Fatalf("%s: error = %+v, want REACTION_EMOJI_UNSUPPORTED", name, ce)
		}
		if !strings.Contains(ce.Message, "7.6.3") || !strings.Contains(ce.Message, "7.7") {
			t.Errorf("%s: message %q should name the server and the needed version", name, ce.Message)
		}
		if !strings.Contains(strings.Join(ce.NextSteps, " "), "--emoji thumbsup") {
			t.Errorf("%s: next_steps %v should offer thumbsup", name, ce.NextSteps)
		}
	}
	if len(writes) != 0 {
		t.Fatalf("writes = %v, want none", writes)
	}
}

// Cloud has no reaction API, so it is refused from the registry without a
// request; offlineClient would fail on dial if one were sent.
func TestCommentReactionCloudUnsupported(t *testing.T) {
	if capabilitySupportFor(CapCommentReactions, FlavorCloud).Supported() {
		t.Fatal("registry marks Cloud reactions supported")
	}
	req := ReactPRCommentReq{Repo: RepoRef{Workspace: "ws", Slug: "repo"}, PRID: 7, ID: 42, Emoji: "thumbsup"}
	c := offlineClient(FlavorCloud)
	_, err := c.DescribeWrite(context.Background(), req)
	if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "REACTION_UNSUPPORTED" {
		t.Fatalf("preview error = %v, want REACTION_UNSUPPORTED", err)
	}
	_, err = c.ReactPRComment(context.Background(), req)
	if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "REACTION_UNSUPPORTED" {
		t.Fatalf("live error = %v, want REACTION_UNSUPPORTED", err)
	}
}

func TestCommentReactionRejectsEmojiCharacters(t *testing.T) {
	for _, emoji := range []string{"", "  ", "\U0001F44D", "thumbs up", "::"} {
		_, err := offlineClient(FlavorDataCenter).DescribeWrite(context.Background(),
			ReactPRCommentReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42, Emoji: emoji})
		if ce := cerrors.AsCLIError(err); ce == nil || ce.Code != "REACTION_BAD_EMOJI" {
			t.Errorf("emoji %q: error = %v, want REACTION_BAD_EMOJI", emoji, err)
		}
	}
}

// A refused reaction keeps the server's reason, category and status, and
// gains guidance that points at the comment listing.
func TestCommentReactionRefusalKeepsServerMessage(t *testing.T) {
	for _, tc := range []struct{ version, emoji, hint string }{
		{"8.19.0", "laugh", "short name"},
		{"7.6.3", "thumbsup", "own comment"},
	} {
		c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				_, _ = fmt.Fprintf(w, `{"version":%q}`, tc.version)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"message":"server says no"}]}`))
		}))
		_, err := c.ReactPRComment(context.Background(), ReactPRCommentReq{
			Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7, ID: 42, Emoji: tc.emoji})
		ce := cerrors.AsCLIError(err)
		if ce == nil || ce.HTTPStatus != http.StatusBadRequest || !strings.Contains(ce.Message, "server says no") {
			t.Fatalf("%s: error = %+v, want the server's 400 message", tc.version, ce)
		}
		if !strings.Contains(ce.Hint, tc.hint) {
			t.Errorf("%s: hint %q should mention %q", tc.version, ce.Hint, tc.hint)
		}
		if !strings.Contains(strings.Join(ce.NextSteps, " "), "comment list --pr PROJ/repo/7") {
			t.Errorf("%s: next_steps %v should point at comment list", tc.version, ce.NextSteps)
		}
	}
}

// A reaction that fails after it was sent may have been applied, so it follows
// the write rule like every other write: not retryable, and pointing at the
// comment listing, which shows each comment's reactions.
func TestCommentReactionFailureIsUnknownOutcome(t *testing.T) {
	for _, tc := range []struct {
		version string
		remove  bool
		method  string
	}{
		{"8.19.0", false, http.MethodPut},
		{"8.19.0", true, http.MethodDelete},
		{"7.6.3", false, http.MethodPost},
	} {
		t.Run(tc.method+"/"+tc.version, func(t *testing.T) {
			var sent string
			c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"version":%q}`, tc.version)
					return
				}
				sent = r.Method
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			repo := RepoRef{Workspace: "PROJ", Slug: "repo"}
			_, err := c.ReactPRComment(context.Background(), ReactPRCommentReq{
				Repo: repo, PRID: 7, ID: 42, Emoji: "thumbsup", Remove: tc.remove})
			if sent != tc.method {
				t.Fatalf("sent %q, want %s", sent, tc.method)
			}
			ce := cerrors.AsCLIError(err)
			target := reactionWriteTarget(repo, 7, 42, "thumbsup")
			if ce == nil || ce.Retryable || ce.HTTPStatus != http.StatusServiceUnavailable {
				t.Fatalf("error = %+v, want a non-retryable 503", ce)
			}
			if !strings.HasPrefix(ce.Message, "Write outcome is unknown for "+target.Description) {
				t.Fatalf("message %q does not name the reaction", ce.Message)
			}
			if len(ce.NextSteps) == 0 || ce.NextSteps[0] != target.ReadSteps[0] {
				t.Fatalf("next steps %q, want the comment listing %q", ce.NextSteps, target.ReadSteps)
			}
		})
	}
}

// dcReactionProps is the properties block a Data Center 7.17 server returned
// for a reacted comment (names replaced): likedBy mirrors the thumbsup users.
const dcReactionProps = `{
  "repositoryId": 9,
  "reactions": [
    {"emoticon": {"shortcut": "thumbsup", "url": "https://bb.example/s/twemoji/12.1.2/1f44d.svg"},
     "users": [{"name": "alice", "slug": "alice", "displayName": "Alice", "id": 1, "type": "NORMAL"}]},
    {"emoticon": {"shortcut": "white_check_mark", "url": "https://bb.example/s/twemoji/12.1.2/2705.svg"},
     "users": [{"name": "alice", "slug": "alice", "id": 1}, {"name": "bob", "slug": "bob", "id": 2}]}
  ],
  "likedBy": {"total": 1, "likers": [{"name": "alice", "slug": "alice", "id": 1}]}
}`

func TestMapDCReactions(t *testing.T) {
	got := mapDCReactions([]byte(dcReactionProps))
	if len(got) != 2 {
		t.Fatalf("reactions = %+v, want thumbsup and white_check_mark only (likedBy must not count twice)", got)
	}
	if got[0].Emoji != "thumbsup" || got[0].Count != 1 || len(got[0].Users) != 1 || got[0].Users[0].Name != "alice" {
		t.Errorf("thumbsup = %+v", got[0])
	}
	if got[1].Emoji != "white_check_mark" || got[1].Count != 2 || got[1].Users[1].Name != "bob" {
		t.Errorf("white_check_mark = %+v", got[1])
	}

	// A server without reactions (before 7.7) only has the like.
	likes := mapDCReactions([]byte(`{"likedBy": {"total": 3, "likers": [{"name": "alice"}, {"name": "bob"}]}}`))
	if len(likes) != 1 || likes[0].Emoji != likeEmoticon || likes[0].Count != 3 || len(likes[0].Users) != 2 {
		t.Errorf("likes only = %+v, want one thumbsup counting the total", likes)
	}

	// A like with no matching thumbsup reaction still shows, merged by user.
	merged := mapDCReactions([]byte(`{
	  "reactions": [{"emoticon": {"shortcut": "eyes"}, "users": [{"slug": "carol"}]},
	                {"emoticon": {"shortcut": "thumbsup"}, "users": [{"slug": "alice"}]}],
	  "likedBy": {"total": 2, "likers": [{"slug": "alice"}, {"slug": "bob"}]}}`))
	if len(merged) != 2 || merged[0].Emoji != "eyes" || merged[0].Count != 1 ||
		merged[1].Emoji != "thumbsup" || merged[1].Count != 2 || len(merged[1].Users) != 2 ||
		merged[1].Users[1].Slug != "bob" {
		t.Errorf("merged likes = %+v, want eyes 1 and thumbsup alice+bob", merged)
	}
	onlyEyes := mapDCReactions([]byte(`{
	  "reactions": [{"emoticon": {"shortcut": "eyes"}, "users": [{"slug": "carol"}]}],
	  "likedBy": {"total": 1, "likers": [{"slug": "bob"}]}}`))
	if len(onlyEyes) != 2 || onlyEyes[1].Emoji != "thumbsup" || onlyEyes[1].Users[0].Slug != "bob" {
		t.Errorf("eyes plus a like = %+v, want the like kept as thumbsup", onlyEyes)
	}

	// One user with fewer fields in one list is still one user.
	alias := mapDCReactions([]byte(`{
	  "reactions": [{"emoticon": {"shortcut": "thumbsup"}, "users": [{"id": 5, "slug": "alice"}]}],
	  "likedBy": {"total": 1, "likers": [{"id": 5}, {"name": "bob", "slug": "bob"}, {"name": "bob"}]}}`))
	if len(alias) != 1 || alias[0].Count != 2 || len(alias[0].Users) != 2 || alias[0].Users[1].Slug != "bob" {
		t.Errorf("aliases = %+v, want thumbsup from alice and bob", alias)
	}

	// Each user counts once, and a null user is no one.
	dup := mapDCReactions([]byte(`{"reactions": [
	  {"emoticon": {"shortcut": "heart"}, "users": [{"slug": "alice"}, null, {"slug": "alice"}]},
	  {"emoticon": {"shortcut": "heart"}, "users": [{"slug": "bob"}]}]}`))
	if len(dup) != 1 || dup[0].Count != 2 || len(dup[0].Users) != 2 {
		t.Errorf("duplicates = %+v, want one heart from alice and bob", dup)
	}

	for name, raw := range map[string]string{
		"absent": ``, "null": `null`, "repository only": `{"repositoryId": 9}`,
		"no likes": `{"likedBy": {"total": 0, "likers": []}}`, "unexpected shape": `{"reactions": "many"}`,
		"null user":   `{"reactions": [{"emoticon": {"shortcut": "eyes"}, "users": [null]}]}`,
		"empty users": `{"reactions": [{"emoticon": {"shortcut": "eyes"}, "users": []}]}`,
		"no users":    `{"reactions": [{"emoticon": {"shortcut": "eyes"}, "users": null}]}`,
		"blank user":  `{"reactions": [{"emoticon": {"shortcut": "eyes"}, "users": [{}]}]}`,
	} {
		if got := mapDCReactions([]byte(raw)); got != nil {
			t.Errorf("%s: reactions = %+v, want none", name, got)
		}
	}
}

// Reactions reach every comment the activity stream carries, replies
// included, and a shape the client cannot read never fails the listing.
func TestListPRCommentsCarriesReactions(t *testing.T) {
	c := newWriteTestClient(t, FlavorDataCenter, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"isLastPage": true, "values": [
		  {"action": "COMMENTED", "comment": {"id": 1, "text": "root", "properties": %s,
		    "comments": [{"id": 2, "text": "reply", "properties": {"reactions": [
		      {"emoticon": {"shortcut": "eyes"}, "users": [{"name": "bob"}]}]}}]}},
		  {"action": "COMMENTED", "comment": {"id": 3, "text": "odd", "properties": {"reactions": {"x": 1}}}}
		]}`, dcReactionProps)
	}))
	res, err := c.ListPRComments(context.Background(), ListPRCommentsOpts{
		Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, PRID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(res.Items))
	}
	if r := res.Items[0].Reactions; len(r) != 2 || r[1].Count != 2 {
		t.Errorf("root reactions = %+v", r)
	}
	if r := res.Items[1].Reactions; len(r) != 1 || r[0].Emoji != "eyes" {
		t.Errorf("reply reactions = %+v", r)
	}
	if r := res.Items[2].Reactions; r != nil {
		t.Errorf("unreadable reactions = %+v, want none", r)
	}
}
