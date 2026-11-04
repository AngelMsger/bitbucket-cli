package apiclient

import (
	"context"
	"strconv"
	"strings"
)

// serverVersion is a Bitbucket Data Center release number as the server
// reports it (e.g. "8.19.3"). Only major.minor.patch take part in comparisons;
// a suffix such as "-rc1" is kept in raw and otherwise ignored.
type serverVersion struct {
	raw   string
	parts [3]int
	known bool
}

// parseServerVersion reads the leading numeric fields of raw. A value without
// a numeric major part is unknown, which capability resolution treats as the
// current release.
func parseServerVersion(raw string) serverVersion {
	v := serverVersion{raw: strings.TrimSpace(raw)}
	for i, field := range strings.SplitN(v.raw, ".", 3) {
		end := 0
		for end < len(field) && field[end] >= '0' && field[end] <= '9' {
			end++
		}
		if end == 0 {
			break
		}
		v.parts[i], _ = strconv.Atoi(field[:end])
		v.known = true
		if end < len(field) {
			break
		}
	}
	return v
}

// atLeast reports whether v is the same release as o or a later one.
func (v serverVersion) atLeast(o serverVersion) bool {
	for i := range v.parts {
		if v.parts[i] != o.parts[i] {
			return v.parts[i] > o.parts[i]
		}
	}
	return true
}

func (v serverVersion) String() string { return v.raw }

// dcServerVersion reads the Data Center release number from
// application-properties, the endpoint flavor detection already probes. It is
// a read, so the --dry-run preview of a version-dependent write may call it.
func (c *apiClient) dcServerVersion(ctx context.Context) (serverVersion, error) {
	var props struct {
		Version string `json:"version"`
	}
	if err := c.getJSON(ctx, c.apiBase()+"/application-properties", nil, &props); err != nil {
		return serverVersion{}, err
	}
	return parseServerVersion(props.Version), nil
}
