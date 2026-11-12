package gittransport

import (
	"bytes"
	"context"
	"encoding/pem"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestURLScope(t *testing.T) {
	for _, tt := range []struct {
		raw, base string
		cloud, ok bool
	}{
		{"ssh://git@git.example.com:12345/TEAM/repo.git", "https://git.example.com/bitbucket", false, true},
		{"git@git.example.com:TEAM/repo.git", "https://git.example.com", false, true},
		{"https://git.example.com/bitbucket/scm/TEAM/repo.git", "https://git.example.com/bitbucket", false, true},
		{"https://git.example.com/other/scm/TEAM/repo.git", "https://git.example.com/bitbucket", false, false},
		{"https://git.example.com:443/scm/TEAM/repo.git", "https://git.example.com:8443", false, false},
		{"https://git.example.com:443/scm/TEAM/repo.git", "https://git.example.com", false, true},
		{"https://git.example.com/scm/TEAM/repo.git", "https://git.example.com:443", false, true},
		{"https://evil.example.com/scm/TEAM/repo.git", "https://git.example.com", false, false},
		{"https://git.example.com/scm/%2e%2e/repo.git", "https://git.example.com", false, false},
		{"https://git.example.com/scm/TEAM/repo.git?token=x", "https://git.example.com", false, false},
		{"https://u:secret@git.example.com/scm/TEAM/repo.git", "https://git.example.com", false, false},
		{"http://git.example.com/scm/TEAM/repo.git", "https://git.example.com", false, false},
		{"git@bitbucket.org:team/repo.git", "https://api.bitbucket.org", true, true},
		{"https://bitbucket.org/team/repo.git", "https://api.bitbucket.org", true, true},
	} {
		_, _, ok := Repository(tt.raw, tt.base, tt.cloud)
		if ok != tt.ok {
			t.Errorf("%s: matched=%v", tt.raw, ok)
		}
	}
	for _, bad := range []string{"http://git.example.com/scm/TEAM/repo.git", "https://other.example.com/scm/TEAM/repo.git", "https://git.example.com/scm/TEAM/repo.git?x=1"} {
		if _, err := HTTPS(bad, "https://git.example.com", false); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	got, err := HTTPS("https://alice@git.example.com/scm/TEAM/repo.git", "https://git.example.com", false)
	if err != nil || got != "https://git.example.com/scm/TEAM/repo.git" {
		t.Fatalf("username removal: %s %v", got, err)
	}
	if _, err := HTTPS("https://git.example.com:443/scm/TEAM/repo.git", "https://git.example.com", false); err != nil {
		t.Fatal("default HTTPS port rejected:", err)
	}
}

func TestBoundedCommand(t *testing.T) {
	for _, args := range [][]string{{"fetch", "--all"}, {"clone", "url", "dir", "--recurse-submodules"}, {"ls-remote", "url", "--upload-pack=evil"}, {"clone", "--recurse-submodules", "url"}, {"ls-remote", "--upload-pack=evil", "url"}, {"push"}, {"checkout", "main"}, {"-c", "http.followRedirects=true", "ls-remote", "url"}, {"-c", "trace2.eventTarget=/tmp/log", "ls-remote", "url"}} {
		if _, err := Parse(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	c, err := Parse([]string{"-C", "work", "fetch", "--quiet", "--prune", "upstream"})
	if err != nil || c.Target != "upstream" || c.Operation != "fetch" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestRedactionAcrossWrites(t *testing.T) {
	var out bytes.Buffer
	w := &redactedWriter{to: &out, replace: strings.NewReplacer("private-token", "[redacted]")}
	for _, chunk := range []string{"remote: private-", "token\nprivate-to", "ken"} {
		_, _ = w.Write([]byte(chunk))
	}
	w.flush()
	if out.String() != "remote: [redacted]\n[redacted]" {
		t.Fatal(out.String())
	}
}

func TestHTTPSCloneFetchAndReadWithoutSSH(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bare := filepath.Join(root, "repo.git")
	author := filepath.Join(root, "author")
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(gitPath, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--bare", bare)
	git("init", "-b", "main", author)
	commit := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(author, "README"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		git("-C", author, "add", "README")
		git("-C", author, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", text)
		git("-C", author, "push", bare, "main")
	}
	commit("one")
	git("--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main")
	backend := &cgi.Handler{Path: gitPath, Args: []string{"http-backend"}, Dir: root, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	const token = "synthetic-private-pat"
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		calls++
		backend.ServeHTTP(w, r)
	}))
	defer server.Close()
	ca := filepath.Join(root, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSL_CAINFO", ca)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "empty-config"))
	trace := filepath.Join(root, "trace.json")
	git("config", "--global", "trace2.eventTarget", trace)
	git("config", "--global", "trace2.envVars", "GIT_CONFIG_VALUE_2")
	t.Setenv("GIT_SSH_COMMAND", "false")
	t.Setenv("GIT_TRACE_CURL", "1")
	original := "ssh://git@git.example.com:12345/TEAM/repo.git"
	a := &Authentication{Original: original, HTTPS: server.URL + "/repo.git", Header: "Bearer " + token, Secret: token}
	run := func(args ...string) string {
		t.Helper()
		c, err := Parse(args)
		if err != nil {
			t.Fatal(err)
		}
		var out, stderr bytes.Buffer
		code, err := Run(context.Background(), c, a, &out, &stderr)
		if code != 0 || err != nil {
			t.Fatalf("run %v: %d %v %s", args, code, err, stderr.String())
		}
		if strings.Contains(out.String()+stderr.String(), token) {
			t.Fatal("credential leaked")
		}
		return out.String()
	}
	target := filepath.Join(root, "checkout")
	run("ls-remote", "--heads", original)
	run("clone", "--quiet", "--origin", "upstream", original, target)
	if got := git("-C", target, "config", "--get", "remote.upstream.url"); got != original {
		t.Fatalf("rewrote stored URL: %s", got)
	}
	commit("two")
	run("-C", target, "fetch", "--quiet", "upstream")
	if got, want := git("-C", target, "rev-parse", "upstream/main"), git("-C", author, "rev-parse", "HEAD"); got != want {
		t.Fatal("fetch did not advance remote branch")
	}
	cfg, err := os.ReadFile(filepath.Join(target, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), token) || strings.Contains(string(cfg), "insteadOf") || strings.Contains(string(cfg), "extraHeader") {
		t.Fatal("authentication persisted")
	}
	traced, _ := os.ReadFile(trace)
	if strings.Contains(string(traced), token) {
		t.Fatal("Git trace configuration leaked the token")
	}
	if calls < 3 {
		t.Fatal("Git did not authenticate")
	}
	// Expired tokens retain Git's failure and do not invoke SSH or a credential helper.
	bad := *a
	bad.Header = "Bearer rejected"
	c, _ := Parse([]string{"ls-remote", original})
	var out, stderr bytes.Buffer
	code, err := Run(context.Background(), c, &bad, &out, &stderr)
	if err != nil || code == 0 {
		t.Fatalf("rejected token: %d %v", code, err)
	}
}

func TestConfiguredRedirectCannotEscapeRepo(t *testing.T) {
	var forwarded string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repo.git/info/refs" {
			http.Redirect(w, r, "/another-deployment/collect", http.StatusFound)
			return
		}
		forwarded = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSL_CAINFO", ca)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	target := server.URL + "/repo.git"
	if out, err := exec.Command("git", "config", "--global", "http."+target+".followRedirects", "true").CombinedOutput(); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	a := &Authentication{Original: "ssh://git@example.com/TEAM/repo.git", HTTPS: target, Header: "Bearer synthetic-review-token", Secret: "synthetic-review-token"}
	c, err := Parse([]string{"ls-remote", a.Original})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	_, err = Run(context.Background(), c, a, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if forwarded != "" {
		t.Fatalf("repository credential followed redirect outside scoped path: %s", forwarded)
	}
}
