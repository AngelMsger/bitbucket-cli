// Command mockserver is a minimal in-memory Bitbucket Data Center REST API,
// used by scripts/e2e.sh to exercise bitbucket-cli end-to-end without a real
// server. It prints its base URL on the first line of stdout, then serves.
//
// The mock covers the Data Center (REST 1.0) endpoints the e2e flow touches:
// application-properties probe, repos, pull requests (list / get / diff /
// commits / activities / comments / approve), branches, commits, plus a
// `/releases/latest` route for the update check.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mockserver: listen failed:", err)
		os.Exit(1)
	}
	fmt.Printf("http://%s\n", ln.Addr().String())
	_ = os.Stdout.Sync()

	if err := http.Serve(ln, routes()); err != nil {
		fmt.Fprintln(os.Stderr, "mockserver:", err)
		os.Exit(1)
	}
}

func routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/api/1.0/application-properties", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"version": "8.19.0", "buildNumber": "0", "displayName": "Bitbucket"})
	})

	// Workspace / project discovery.
	mux.HandleFunc("GET /rest/api/1.0/projects", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{
					"key": "PROJ", "name": "Demo project", "id": 1,
					"description": "Demo", "public": false, "type": "NORMAL",
					"links": map[string]any{"self": []any{map[string]string{"href": "https://bitbucket.example.com/projects/PROJ"}}},
				},
			},
			"size": 1, "limit": 25, "start": 0, "isLastPage": true,
		})
	})
	mux.HandleFunc("GET /rest/api/1.0/projects/{key}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"key": r.PathValue("key"), "name": "Demo project", "id": 1,
			"description": "Demo", "public": false, "type": "NORMAL",
		})
	})

	// User discovery.
	mux.HandleFunc("GET /rest/api/1.0/users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []any{user()},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET /rest/api/1.0/users/{slug}", func(w http.ResponseWriter, r *http.Request) {
		u := user()
		u["slug"] = r.PathValue("slug")
		u["name"] = r.PathValue("slug")
		writeJSON(w, u)
	})

	// Tags.
	mux.HandleFunc("GET /rest/api/1.0/projects/{key}/repos/{slug}/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{
					"id": "refs/tags/v1.2.3", "displayId": "v1.2.3",
					"type": "TAG", "latestCommit": "aaaa111",
				},
			},
			"size": 1, "limit": 25, "start": 0, "isLastPage": true,
		})
	})
	mux.HandleFunc("GET /rest/api/1.0/projects/{key}/repos/{slug}/tags/{name}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": "refs/tags/" + r.PathValue("name"), "displayId": r.PathValue("name"),
			"type": "TAG", "latestCommit": "aaaa111",
		})
	})

	mux.HandleFunc("GET /rest/api/1.0/projects/{key}/repos", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		writeJSON(w, map[string]any{
			"values":     []any{repo(key, "demo", "Demo")},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET /rest/api/1.0/projects/{key}/repos/{slug}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, repo(r.PathValue("key"), r.PathValue("slug"), "Demo"))
	})

	prKey := "/rest/api/1.0/projects/{key}/repos/{slug}/pull-requests"
	mux.HandleFunc("GET "+prKey, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []any{pr(1, "Add login flow", "OPEN")},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET "+prKey+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := atoi(r.PathValue("id"))
		if id == 404 {
			http.Error(w, `{"errors":[{"message":"Pull request not found"}]}`, http.StatusNotFound)
			return
		}
		writeJSON(w, pr(id, "Add login flow", "OPEN"))
	})
	mux.HandleFunc("GET "+prKey+"/{id}/diff", func(w http.ResponseWriter, r *http.Request) {
		// PR 2 emulates a Data Center instance that returns a JSON hunk model at
		// the diff endpoint (ignoring Accept: text/plain) — the case that used to
		// break inline comments. PR 1 keeps the plain-text path.
		if r.PathValue("id") == "2" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(jsonDiff("src/server.go")))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("--- a/src/server.go\n+++ b/src/server.go\n@@ -1 +1 @@\n-old\n+new\n"))
	})
	mux.HandleFunc("GET "+prKey+"/{id}/commits", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []any{commit("aaaa111", "feat: add login flow")},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET "+prKey+"/{id}/activities", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{"id": 100, "action": "COMMENTED", "user": user(), "createdDate": 1, "comment": map[string]any{
					"id": 9001, "text": "Looks good", "author": user(), "createdDate": 1, "version": 0,
					"state": "OPEN", "severity": "NORMAL",
					// DC nests replies under the root comment rather than emitting
					// them as separate activity entries.
					"comments": []any{
						map[string]any{
							"id": 9011, "text": "Thanks for the review", "author": user(),
							"createdDate": 2, "version": 0, "state": "OPEN", "severity": "NORMAL",
						},
					},
				}},
				// Inline comments: DC carries the anchor on the activity
				// (commentAnchor), a sibling of comment — not inside it.
				map[string]any{"id": 102, "action": "COMMENTED", "user": user(), "createdDate": 3,
					"commentAnchor": map[string]any{"path": "src/app.go", "line": 10, "lineType": "CONTEXT", "fileType": "TO"},
					"comment": map[string]any{
						"id": 9002, "text": "Already fixed", "author": user(), "createdDate": 3, "version": 0,
						"state": "RESOLVED", "severity": "NORMAL",
					}},
				map[string]any{"id": 103, "action": "COMMENTED", "user": user(), "createdDate": 4,
					"commentAnchor": map[string]any{"path": "src/app.go", "line": 20, "lineType": "ADDED", "fileType": "TO"},
					"comment": map[string]any{
						"id": 9003, "text": "Please rename this", "author": user(), "createdDate": 4, "version": 0,
						"state": "OPEN", "severity": "BLOCKER",
					}},
				map[string]any{"id": 101, "action": "APPROVED", "user": user(), "createdDate": 2},
			},
			"size": 2, "limit": 25, "start": 0, "isLastPage": true,
		})
	})
	mux.HandleFunc("POST "+prKey+"/{id}/comments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"id": 9100, "text": "added", "author": user(), "createdDate": 1, "version": 0,
		})
	})
	mux.HandleFunc("POST "+prKey+"/{id}/approve", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("DELETE "+prKey+"/{id}/approve", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	branchKey := "/rest/api/1.0/projects/{key}/repos/{slug}/branches"
	mux.HandleFunc("GET "+branchKey, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{"id": "refs/heads/main", "displayId": "main", "type": "BRANCH",
					"latestCommit": "aaaa111", "isDefault": true},
			},
			"size": 1, "limit": 25, "start": 0, "isLastPage": true,
		})
	})

	commitKey := "/rest/api/1.0/projects/{key}/repos/{slug}/commits"
	mux.HandleFunc("GET "+commitKey, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []any{commit("aaaa111", "feat: add login flow")},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET "+commitKey+"/{hash}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, commit(r.PathValue("hash"), "feat: add login flow"))
	})

	// v0.2: source browsing + PR file-level + merge check + build status.
	filesRoot := "/rest/api/1.0/projects/{key}/repos/{slug}/files"
	mux.HandleFunc("GET "+filesRoot, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []string{"README.md", "src/server.go", "src/cli.go"},
			"size":       3,
			"limit":      1000,
			"start":      0,
			"isLastPage": true,
		})
	})
	mux.HandleFunc("GET "+filesRoot+"/{path...}", func(w http.ResponseWriter, r *http.Request) {
		p := r.PathValue("path")
		writeJSON(w, map[string]any{
			"values":     []string{p + "/hello.go", p + "/world.go"},
			"size":       2,
			"limit":      1000,
			"start":      0,
			"isLastPage": true,
		})
	})
	rawRoot := "/rest/api/1.0/projects/{key}/repos/{slug}/raw"
	mux.HandleFunc("GET "+rawRoot+"/{path...}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("line 1\nline 2\nline 3\nline 4\nline 5\n"))
		_ = r
	})

	mux.HandleFunc("GET "+prKey+"/{id}/changes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{
					"path":     map[string]string{"toString": "src/server.go"},
					"type":     "MODIFY",
					"nodeType": "FILE",
				},
				map[string]any{
					"path":     map[string]string{"toString": "README.md"},
					"type":     "ADD",
					"nodeType": "FILE",
				},
			},
			"size": 2, "limit": 100, "start": 0, "isLastPage": true,
		})
	})
	mux.HandleFunc("GET "+prKey+"/{id}/diff/{path...}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "2" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(jsonDiff(r.PathValue("path"))))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("--- a/" + r.PathValue("path") + "\n+++ b/" + r.PathValue("path") + "\n@@ -1 +1 @@\n-old\n+new\n"))
	})
	mux.HandleFunc("GET "+prKey+"/{id}/merge", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"canMerge":   false,
			"conflicted": true,
			"outcome":    "CONFLICTED",
			"vetoes": []any{
				map[string]string{"summaryMessage": "Conflicts in src/server.go", "detailedMessage": "Resolve before merging."},
			},
		})
	})
	mux.HandleFunc("GET /rest/build-status/1.0/commits/{hash}", func(w http.ResponseWriter, r *http.Request) {
		hash := r.PathValue("hash")
		writeJSON(w, map[string]any{
			"values": []any{
				map[string]any{
					"key":         "ci/build",
					"name":        "build",
					"state":       "SUCCESSFUL",
					"url":         "https://ci.example.com/builds/1",
					"description": "Built " + hash,
					"dateAdded":   1,
				},
			},
			"size": 1, "isLastPage": true,
		})
	})

	// v0.2.1: cross-repo PR inbox.
	mux.HandleFunc("GET /rest/api/1.0/dashboard/pull-requests", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"values":     []any{pr(7, "Wire payment retry", "OPEN")},
			"size":       1,
			"limit":      25,
			"start":      0,
			"isLastPage": true,
		})
	})

	mux.HandleFunc("GET /releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"tag_name": "v99.0.0", "html_url": "https://example/releases"})
	})

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, fmt.Sprintf(`{"errors":[{"message":"no route for %s %s"}]}`, r.Method, r.URL.Path), http.StatusNotFound)
	}))
	return mux
}

func repo(key, slug, name string) map[string]any {
	return map[string]any{
		"slug":        slug,
		"id":          1,
		"name":        name,
		"description": "Demo repository",
		"public":      false,
		"state":       "AVAILABLE",
		"project":     map[string]any{"key": key, "name": "Demo project", "id": 1},
		"links": map[string]any{
			"clone": []any{
				map[string]any{"href": "https://bitbucket.example.com/scm/" + strings.ToLower(key) + "/" + slug + ".git", "name": "http"},
				map[string]any{"href": "ssh://git@bitbucket.example.com/" + strings.ToLower(key) + "/" + slug + ".git", "name": "ssh"},
			},
			"self": []any{map[string]any{"href": "https://bitbucket.example.com/projects/" + key + "/repos/" + slug + "/browse"}},
		},
	}
}

func pr(id int, title, state string) map[string]any {
	return map[string]any{
		"id":          id,
		"version":     0,
		"title":       title,
		"description": "PR description",
		"state":       state,
		"open":        state == "OPEN",
		"closed":      state != "OPEN",
		"createdDate": 1,
		"updatedDate": 2,
		"fromRef": map[string]any{
			"id": "refs/heads/feature/x", "displayId": "feature/x", "latestCommit": "bbbb222",
			"repository": repo("PROJ", "demo", "Demo"),
		},
		"toRef": map[string]any{
			"id": "refs/heads/main", "displayId": "main", "latestCommit": "aaaa111",
			"repository": repo("PROJ", "demo", "Demo"),
		},
		"author":       map[string]any{"user": user(), "role": "AUTHOR", "approved": false, "status": "UNAPPROVED"},
		"reviewers":    []any{map[string]any{"user": user(), "role": "REVIEWER", "approved": false, "status": "UNAPPROVED"}},
		"participants": []any{},
		"properties":   map[string]any{"commentCount": 1},
		"links":        map[string]any{"self": []any{map[string]any{"href": "https://bitbucket.example.com/pr/" + itoa(id)}}},
	}
}

func commit(hash, message string) map[string]any {
	return map[string]any{
		"id":              hash,
		"displayId":       hash[:min(7, len(hash))],
		"message":         message,
		"authorTimestamp": 1,
		"author":          user(),
		"parents":         []any{},
	}
}

func user() map[string]any {
	return map[string]any{
		"name":         "alice",
		"emailAddress": "alice@example.com",
		"id":           1,
		"displayName":  "Alice Example",
		"active":       true,
		"slug":         "alice",
		"type":         "NORMAL",
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// jsonDiff returns a Bitbucket Data Center JSON hunk model for path: a hunk at
// old/new line 10 removing one line and adding two (new lines 10 and 11). It
// lets the e2e flow exercise the JSON diff path that text-only parsing missed.
func jsonDiff(path string) string {
	return `{"diffs":[{"source":{"toString":"` + path + `"},"destination":{"toString":"` + path + `"},` +
		`"hunks":[{"sourceLine":10,"sourceSpan":2,"destinationLine":10,"destinationSpan":3,"segments":[` +
		`{"type":"CONTEXT","lines":[{"source":10,"destination":10,"line":"old"}]},` +
		`{"type":"REMOVED","lines":[{"source":11,"destination":11,"line":"old"}]},` +
		`{"type":"ADDED","lines":[{"source":12,"destination":11,"line":"new"},{"source":12,"destination":12,"line":"new2"}]}` +
		`]}]}]}`
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
