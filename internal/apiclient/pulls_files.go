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

// ListPRThreads regroups all PR comments into per-file inline threads plus a
// "general discussion" bucket. No new API call — the inner ListPRComments is
// walked to completion and the result reshuffled in Go.
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

	// Build reply chains. roots[id] is a top-level comment; children[parent]
	// holds replies in arrival order.
	byID := map[int]Comment{}
	children := map[int][]Comment{}
	var roots []Comment
	for _, c := range all {
		byID[c.ID] = c
	}
	for _, c := range all {
		if c.ParentID == 0 {
			roots = append(roots, c)
		} else {
			children[c.ParentID] = append(children[c.ParentID], c)
		}
	}

	// For each root, gather the full reply tree (DFS, preserving order).
	type key struct {
		file string
		line int
	}
	threads := map[key]*Thread{}
	var order []key
	for _, root := range roots {
		k := key{}
		if root.Inline != nil {
			k.file = root.Inline.Path
			k.line = root.Inline.Line
		}
		t, ok := threads[k]
		if !ok {
			t = &Thread{File: k.file, Resolved: root.Resolved}
			if root.Inline != nil {
				a := *root.Inline
				t.Anchor = &a
			}
			threads[k] = t
			order = append(order, k)
		}
		t.Comments = append(t.Comments, root)
		var walk func(parentID int)
		walk = func(parentID int) {
			kids := children[parentID]
			for _, kid := range kids {
				t.Comments = append(t.Comments, kid)
				walk(kid.ID)
			}
		}
		walk(root.ID)
	}

	// Sort threads: general discussion last; inline grouped by file then line.
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if (a.file == "") != (b.file == "") {
			return a.file != "" // inline before general
		}
		if a.file != b.file {
			return a.file < b.file
		}
		return a.line < b.line
	})
	out := ListResult[Thread]{}
	for _, k := range order {
		out.Items = append(out.Items, *threads[k])
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
