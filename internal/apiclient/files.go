package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	cerrors "github.com/angelmsger/bitbucket-cli/internal/errors"
)

// ListFiles enumerates entries directly under opt.Path at opt.Ref.
//   Cloud: GET /2.0/repositories/{ws}/{slug}/src/{ref}/{path}?format=meta
//   DC:    GET /rest/api/1.0/projects/{key}/repos/{slug}/files/{path}?at={ref}
//
// Bitbucket DC's `/files` endpoint returns only file paths (not directories
// or sizes), so the resulting FileEntry instances have Type="file" and Size=0
// on DC. Cloud returns rich metadata (type / size / commit) directly.
func (c *apiClient) ListFiles(ctx context.Context, opt FileListOpts) (ListResult[FileEntry], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[FileEntry]{}, err
	}
	gitRef, err := c.resolveRef(ctx, opt.Repo, opt.Ref)
	if err != nil {
		return ListResult[FileEntry]{}, err
	}
	limit := c.limitOf(opt.ListOpts)

	if c.flavor == FlavorCloud {
		q := url.Values{}
		q.Set("format", "meta")
		q.Set("pagelen", itoa(limit))
		endpoint := c.srcPath(opt.Repo, gitRef, opt.Path)
		if cloudFollowURL(opt.Cursor) {
			endpoint = opt.Cursor
			q = nil
		}
		var raw struct {
			Values []struct {
				Path   string `json:"path"`
				Type   string `json:"type"` // "commit_file" | "commit_directory" | …
				Size   int64  `json:"size"`
				Commit struct {
					Hash string `json:"hash"`
				} `json:"commit"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if err := c.getJSON(ctx, endpoint, q, &raw); err != nil {
			return ListResult[FileEntry]{}, err
		}
		res := ListResult[FileEntry]{Next: cloudNextCursor(raw.Next)}
		for _, v := range raw.Values {
			res.Items = append(res.Items, FileEntry{
				Path:   v.Path,
				Name:   path.Base(v.Path),
				Type:   cloudFileType(v.Type),
				Size:   v.Size,
				Commit: v.Commit.Hash,
			})
		}
		return res, nil
	}
	// Data Center: returns paginated string paths.
	q := c.queryWithLimit(opt.Cursor, limit)
	q.Set("at", gitRef)
	var raw struct {
		Values     []string `json:"values"`
		Size       int      `json:"size"`
		Limit      int      `json:"limit"`
		Start      int      `json:"start"`
		IsLastPage bool     `json:"isLastPage"`
	}
	if err := c.getJSON(ctx, c.filesPath(opt.Repo, opt.Path), q, &raw); err != nil {
		return ListResult[FileEntry]{}, err
	}
	res := ListResult[FileEntry]{
		Next: nextOffsetToken(opt.Cursor, limit, len(raw.Values), raw.IsLastPage),
	}
	for _, p := range raw.Values {
		res.Items = append(res.Items, FileEntry{
			Path: joinPath(opt.Path, p),
			Name: path.Base(p),
			Type: "file",
		})
	}
	return res, nil
}

// GetFile fetches the raw bytes of a single file at opt.Ref.
//   Cloud: GET /2.0/repositories/{ws}/{slug}/src/{ref}/{path}        (raw body)
//   DC:    GET /rest/api/1.0/projects/{key}/repos/{slug}/raw/{path}?at={ref}
func (c *apiClient) GetFile(ctx context.Context, opt FileGetOpts) (*FileContent, error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return nil, err
	}
	if opt.Path == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "FILE_NO_PATH",
			"a file --path is required")
	}
	gitRef, err := c.resolveRef(ctx, opt.Repo, opt.Ref)
	if err != nil {
		return nil, err
	}
	var endpoint string
	var query url.Values
	if c.flavor == FlavorCloud {
		endpoint = c.srcPath(opt.Repo, gitRef, opt.Path)
	} else {
		endpoint = c.rawPath(opt.Repo, opt.Path)
		query = url.Values{}
		query.Set("at", gitRef)
	}
	body, err := c.getRaw(ctx, endpoint, query)
	if err != nil {
		return nil, err
	}
	enc := "utf-8"
	if !utf8.Valid(body) {
		enc = "binary"
	}
	return &FileContent{
		Path:     opt.Path,
		Ref:      gitRef,
		Bytes:    body,
		Size:     int64(len(body)),
		Encoding: enc,
	}, nil
}

// Tree walks the repository tree from opt.Path down to opt.Depth levels,
// returning all files and (optionally) directories encountered. Depth=0 means
// unlimited. A non-existent path surfaces a NotFound error from the upstream.
//
// On Cloud the `values` listing carries the entry type, so directories are
// recursed in-process. On DC the `/files` listing flattens all files under
// the requested root into a single paginated string list, so the depth filter
// is applied client-side.
func (c *apiClient) Tree(ctx context.Context, opt TreeOpts) (ListResult[FileEntry], error) {
	if err := checkRepoRef(opt.Repo); err != nil {
		return ListResult[FileEntry]{}, err
	}
	gitRef, err := c.resolveRef(ctx, opt.Repo, opt.Ref)
	if err != nil {
		return ListResult[FileEntry]{}, err
	}
	out := ListResult[FileEntry]{}
	if c.flavor == FlavorCloud {
		err := c.cloudWalk(ctx, opt.Repo, gitRef, opt.Path, opt.Depth, 0, &out.Items)
		return out, err
	}
	// DC: one paginated request returns every file at and below opt.Path.
	cursor := ""
	for {
		page, err := c.ListFiles(ctx, FileListOpts{
			ListOpts: ListOpts{Limit: 250, Cursor: cursor},
			Repo:     opt.Repo, Ref: gitRef, Path: opt.Path,
		})
		if err != nil {
			return out, err
		}
		for _, e := range page.Items {
			if opt.Depth > 0 && depthBelow(opt.Path, e.Path) > opt.Depth {
				continue
			}
			out.Items = append(out.Items, e)
		}
		if page.Next == "" {
			return out, nil
		}
		cursor = page.Next
	}
}

func (c *apiClient) cloudWalk(ctx context.Context, repo RepoRef, gitRef, p string, depthLimit, depthSoFar int, dst *[]FileEntry) error {
	cursor := ""
	for {
		page, err := c.ListFiles(ctx, FileListOpts{
			ListOpts: ListOpts{Limit: 100, Cursor: cursor},
			Repo:     repo, Ref: gitRef, Path: p,
		})
		if err != nil {
			return err
		}
		for _, e := range page.Items {
			*dst = append(*dst, e)
			if e.Type == "dir" && (depthLimit == 0 || depthSoFar+1 < depthLimit) {
				if err := c.cloudWalk(ctx, repo, gitRef, e.Path, depthLimit, depthSoFar+1, dst); err != nil {
					return err
				}
			}
		}
		if page.Next == "" {
			return nil
		}
		cursor = page.Next
	}
}

// resolveRef returns the effective git ref. An empty ref defaults to the
// repository's main / default branch, which is one extra GetRepository call.
func (c *apiClient) resolveRef(ctx context.Context, repo RepoRef, ref string) (string, error) {
	if strings.TrimSpace(ref) != "" {
		return ref, nil
	}
	r, err := c.GetRepository(ctx, repo)
	if err != nil {
		return "", err
	}
	if r.DefaultBranch != "" {
		return r.DefaultBranch, nil
	}
	if r.MainBranch != "" {
		return r.MainBranch, nil
	}
	return "main", nil
}

// getRaw performs a GET expecting a non-JSON body (raw file content).
func (c *apiClient) getRaw(ctx context.Context, endpoint string, query url.Values) ([]byte, error) {
	full := c.absEndpoint(endpoint, query)
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REQUEST", "failed to build request")
	}
	req.Header.Set("Accept", "application/octet-stream, text/plain, */*")
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryNetwork, "NETWORK", "request to "+full+" failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, c.httpError(resp)
	}
	return io.ReadAll(resp.Body)
}

func cloudFileType(t string) string {
	switch t {
	case "commit_file":
		return "file"
	case "commit_directory":
		return "dir"
	case "commit_link":
		return "link"
	case "commit_submodule":
		return "submodule"
	}
	return t
}

// depthBelow returns how many path components `full` extends past `base`.
// joinPath / Clean normalize trailing slashes so the result is stable.
func depthBelow(base, full string) int {
	if base == "" {
		return len(strings.Split(strings.Trim(full, "/"), "/"))
	}
	rel := strings.TrimPrefix(strings.Trim(full, "/"), strings.Trim(base, "/")+"/")
	if rel == "" {
		return 0
	}
	return len(strings.Split(rel, "/"))
}

// joinPath joins two path components, tolerating empty segments.
func joinPath(a, b string) string {
	a, b = strings.Trim(a, "/"), strings.Trim(b, "/")
	switch {
	case a == "" && b == "":
		return ""
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "/" + b
}

func itoa(n int) string {
	// localized helper to avoid importing strconv across mixed callers
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
