package apiclient

import (
	"context"
	"net/url"
	"sort"
	"strings"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListPRFiles returns the per-file diffstat of a PR — purely metadata, no
// patch bytes. Use this before fetching individual file diffs to budget
// context.
//
//	Cloud: GET /2.0/repositories/{ws}/{slug}/pullrequests/{id}/diffstat
//	DC:    GET /rest/api/1.0/projects/{key}/repos/{slug}/pull-requests/{id}/changes
func (c *apiClient) ListPRFiles(ctx context.Context, repo RepoRef, id int) (ListResult[Diffstat], error) {
	if err := checkRepoRef(repo); err != nil {
		return ListResult[Diffstat]{}, err
	}
	if c.flavor == FlavorCloud {
		var raw struct {
			Values []struct {
				Status       string `json:"status"`
				LinesAdded   int    `json:"lines_added"`
				LinesRemoved int    `json:"lines_removed"`
				Old          *struct {
					Path string `json:"path"`
				} `json:"old"`
				New *struct {
					Path string `json:"path"`
				} `json:"new"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, c.prDiffstatPath(repo, id), nil, &raw); err != nil {
			return ListResult[Diffstat]{}, err
		}
		res := ListResult[Diffstat]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			d := Diffstat{
				Status:       normalizeDiffstatStatus(v.Status),
				LinesAdded:   v.LinesAdded,
				LinesRemoved: v.LinesRemoved,
			}
			if v.New != nil {
				d.Path = v.New.Path
			}
			if v.Old != nil {
				d.OldPath = v.Old.Path
			}
			if d.Path == "" && d.OldPath != "" {
				d.Path = d.OldPath
			}
			res.Items = append(res.Items, d)
		}
		sortDiffstat(res.Items)
		return res, nil
	}
	// Data Center: /changes returns paginated change records (no line counts;
	// callers wanting line counts must fall back to /diff for the full patch).
	var raw struct {
		Values []struct {
			Path struct {
				ToString string `json:"toString"`
			} `json:"path"`
			SrcPath *struct {
				ToString string `json:"toString"`
			} `json:"srcPath"`
			Type       string `json:"type"` // ADD | MODIFY | DELETE | RENAME | COPY
			NodeType   string `json:"nodeType"`
			Executable bool   `json:"executable"`
			ContentID  string `json:"contentId"`
		} `json:"values"`
		IsLastPage bool `json:"isLastPage"`
		Size       int  `json:"size"`
		Limit      int  `json:"limit"`
	}
	q := url.Values{}
	q.Set("limit", "1000")
	if err := c.getJSON(ctx, c.prChangesPath(repo, id), q, &raw); err != nil {
		return ListResult[Diffstat]{}, err
	}
	res := ListResult[Diffstat]{}
	for _, v := range raw.Values {
		d := Diffstat{
			Path:   v.Path.ToString,
			Status: normalizeDiffstatStatus(v.Type),
		}
		if v.SrcPath != nil {
			d.OldPath = v.SrcPath.ToString
		}
		res.Items = append(res.Items, d)
	}
	sortDiffstat(res.Items)
	return res, nil
}

// GetPRDiffByPath returns the unified-diff text for a single file in a PR,
// normalizing a JSON hunk model into text when the server returns one.
//
//	Cloud: GET /2.0/.../pullrequests/{id}/diff?path=<path>
//	DC:    GET /rest/api/1.0/.../pull-requests/{id}/diff/{path}
func (c *apiClient) GetPRDiffByPath(ctx context.Context, repo RepoRef, id int, p string) (string, error) {
	if err := checkRepoRef(repo); err != nil {
		return "", err
	}
	if strings.TrimSpace(p) == "" {
		return "", cerrors.New(cerrors.CategoryUsage, "DIFF_NO_PATH",
			"--path is required for per-file diff")
	}
	endpoint, query := c.prDiffEndpoint(repo, id, p)
	return c.fetchDiffText(ctx, endpoint, query)
}

// ListPRThreads groups all PR comments into review threads — one thread per
// top-level comment plus its reply tree, mirroring how Bitbucket itself models
// a discussion (two independent comments on the same line are two threads, not
// one). No new API call — the inner ListPRComments is walked to completion and
// the result reshuffled in Go, then sorted by file and anchor line with general
// (non-inline) threads last.
func (c *apiClient) ListPRThreads(ctx context.Context, repo RepoRef, id int) (ListResult[Thread], error) {
	if err := checkRepoRef(repo); err != nil {
		return ListResult[Thread]{}, err
	}
	all, err := CollectAll(func(cursor string) (ListResult[Comment], error) {
		return c.ListPRComments(ctx, ListPRCommentsOpts{
			ListOpts: ListOpts{Cursor: cursor},
			Repo:     repo, PRID: id,
		})
	}, 0)
	if err != nil {
		return ListResult[Thread]{}, err
	}

	// Split into top-level comments and replies keyed by their parent.
	children := map[int][]Comment{}
	var roots []Comment
	for _, c := range all {
		if c.ParentID == 0 {
			roots = append(roots, c)
		} else {
			children[c.ParentID] = append(children[c.ParentID], c)
		}
	}

	// One thread per root: append the root, then its reply tree depth-first in
	// arrival order. seen guards against a comment surfacing twice (e.g. an edit
	// re-emitted as a second activity) so it cannot be double-counted.
	seen := map[int]bool{}
	var threads []*Thread
	for _, root := range roots {
		if seen[root.ID] {
			continue
		}
		seen[root.ID] = true
		t := &Thread{Resolved: root.Resolved}
		if root.Inline != nil {
			t.File = root.Inline.Path
			a := *root.Inline
			t.Anchor = &a
		}
		t.Comments = append(t.Comments, root)
		var walk func(parentID int)
		walk = func(parentID int) {
			for _, kid := range children[parentID] {
				if seen[kid.ID] {
					continue
				}
				seen[kid.ID] = true
				t.Comments = append(t.Comments, kid)
				walk(kid.ID)
			}
		}
		walk(root.ID)
		threads = append(threads, t)
	}

	// Sort: inline threads first, grouped by file then anchor line; general
	// (non-inline) threads last. Stable, so same-anchor threads keep arrival
	// order.
	lineOf := func(t *Thread) int {
		if t.Anchor != nil {
			return t.Anchor.Line
		}
		return 0
	}
	sort.SliceStable(threads, func(i, j int) bool {
		a, b := threads[i], threads[j]
		if (a.File == "") != (b.File == "") {
			return a.File != "" // inline before general
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return lineOf(a) < lineOf(b)
	})
	out := ListResult[Thread]{}
	for _, t := range threads {
		out.Items = append(out.Items, *t)
	}
	return out, nil
}

// normalizeDiffstatStatus folds the Cloud and DC status enums into one set.
func normalizeDiffstatStatus(s string) string {
	switch strings.ToLower(s) {
	case "added", "add":
		return "added"
	case "removed", "delete":
		return "removed"
	case "modified", "modify":
		return "modified"
	case "renamed", "rename":
		return "renamed"
	case "copied", "copy":
		return "copied"
	}
	return strings.ToLower(s)
}

// sortDiffstat orders entries so the biggest changes surface first; ties keep
// path order for stability.
func sortDiffstat(ds []Diffstat) {
	sort.SliceStable(ds, func(i, j int) bool {
		si := ds[i].LinesAdded + ds[i].LinesRemoved
		sj := ds[j].LinesAdded + ds[j].LinesRemoved
		if si != sj {
			return si > sj
		}
		return ds[i].Path < ds[j].Path
	})
}
