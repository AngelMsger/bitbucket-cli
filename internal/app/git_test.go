// Full native CLI and optional fx acceptance against a TLS Git smart-HTTP server.
package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/internal/auth"
	"github.com/angelmsger/bitbucket-cli/internal/config"
	"github.com/angelmsger/bitbucket-cli/pkg/constants"
)

func TestGitCLIChild(t *testing.T) {
	if os.Getenv("BITBUCKET_GIT_TEST_CHILD") != "1" {
		return
	}
	for i, a := range os.Args {
		if a == "--" {
			os.Args = append([]string{"bitbucket-cli"}, os.Args[i+1:]...)
			break
		}
	}
	constants.Version = "0.30.0"
	os.Exit(Execute())
}

func TestGitContextSelection(t *testing.T) {
	dir := t.TempDir()
	contexts := []config.NamedContext{
		{Name: "unrelated", BaseURL: "https://other.example.com", Flavor: "datacenter"},
		{Name: "team", BaseURL: "https://git.example.com", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "pat"}},
	}
	if err := config.WriteFile(dir, config.File{CurrentContext: "unrelated", Contexts: contexts}); err != nil {
		t.Fatal(err)
	}
	s := &appState{gflags: globalFlags{configPath: dir}}
	t.Setenv("BITBUCKET_TOKEN", "belongs-to-unrelated")
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	candidate, _, err := s.gitService("git@git.example.com:TEAM/repo.git")
	if err != nil || candidate == nil {
		t.Fatalf("%v %v", candidate, err)
	}
	if candidate.resolved.Secrets.PAT != "" {
		t.Fatal("copied another service's environment token")
	}
	for _, flags := range []globalFlags{{configPath: dir, authScheme: "basic"}, {configPath: dir, flavor: "cloud"}} {
		explicit := &appState{gflags: flags}
		if err := explicit.load(); err != nil {
			t.Fatal(err)
		}
		if candidate, _, err := explicit.gitService("git@git.example.com:TEAM/repo.git"); err != nil || candidate != nil {
			t.Fatal("explicit authentication or flavor escaped to another context", err)
		}
	}
	// PAT usernames are metadata; Basic usernames select an identity.
	contexts = append(contexts, config.NamedContext{Name: "same-pat", BaseURL: "https://git.example.com/", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "pat", Username: "metadata"}})
	if err := config.WriteFile(dir, config.File{CurrentContext: "unrelated", Contexts: contexts}); err != nil {
		t.Fatal(err)
	}
	candidate, _, err = s.gitService("git@git.example.com:TEAM/repo.git")
	if err != nil || candidate == nil {
		t.Fatal("equivalent PAT contexts became ambiguous", err)
	}
	contexts = append(contexts, config.NamedContext{Name: "other-user", BaseURL: "https://git.example.com", Flavor: "datacenter", Auth: config.AuthConfig{Scheme: "basic", Username: "bob"}})
	if err := config.WriteFile(dir, config.File{CurrentContext: "unrelated", Contexts: contexts}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.gitService("git@git.example.com:TEAM/repo.git"); err == nil {
		t.Fatal("ambiguous identity was selected")
	}
	s.gflags.useContext = "unrelated"
	if err := s.load(); err != nil {
		t.Fatal(err)
	}
	candidate, _, err = s.gitService("git@git.example.com:TEAM/repo.git")
	if err != nil || candidate != nil {
		t.Fatal("explicit context escaped")
	}
}

func TestGitCLIHTTPS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX CGI fixture; native Windows acceptance is separate")
	}
	root := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(gitPath, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	host := filepath.Join(root, "host")
	_ = os.MkdirAll(filepath.Join(host, "scm", "TEAM"), 0700)
	makeRepo := func(name string, files map[string]string) string {
		t.Helper()
		author := filepath.Join(root, name+"-author")
		bare := filepath.Join(host, "scm", "TEAM", name+".git")
		runGit("init", "--bare", bare)
		runGit("init", "-b", "main", author)
		for file, text := range files {
			at := filepath.Join(author, file)
			_ = os.MkdirAll(filepath.Dir(at), 0700)
			if err := os.WriteFile(at, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
		runGit("-C", author, "add", ".")
		runGit("-C", author, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
		runGit("-C", author, "push", bare, "main")
		runGit("--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main")
		return author
	}
	repoAuthor := makeRepo("repo", map[string]string{"README": "one\n"})
	const token = "synthetic-pat-for-git-test"
	var base string
	backend := &cgi.Handler{Path: gitPath, Args: []string{"http-backend"}, Dir: host, Env: []string{"GIT_PROJECT_ROOT=" + host, "GIT_HTTP_EXPORT_ALL=1"}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/scm/") {
			backend.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-AUSERNAME", "alice")
		if strings.Contains(r.URL.Path, "/repos/") {
			name := filepath.Base(r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{"slug": name, "name": name, "project": map[string]string{"key": "TEAM"}, "links": map[string]any{"clone": []map[string]string{{"name": "http", "href": base + "/scm/TEAM/" + name + ".git"}, {"name": "ssh", "href": "ssh://git@127.0.0.1:12345/TEAM/" + name + ".git"}}}})
			return
		}
		if strings.Contains(r.URL.Path, "/users/") {
			fmt.Fprint(w, `{"name":"alice","slug":"alice","displayName":"Alice"}`)
			return
		}
		fmt.Fprint(w, `{"version":"9.0.0"}`)
	}))
	defer server.Close()
	base = server.URL
	ca := filepath.Join(root, "ca.pem")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	childEnv := append(os.Environ(), "BITBUCKET_GIT_TEST_CHILD=1", "BITBUCKET_SERVER="+base, "BITBUCKET_FLAVOR=datacenter", "BITBUCKET_TOKEN="+token, "BITBUCKET_AUTH_SCHEME=pat", "BITBUCKET_CLI_NO_UPDATE_NOTIFIER=1", "BITBUCKET_CLI_NO_SKILL_HINT=1", "GIT_SSL_CAINFO="+ca, "SSL_CERT_FILE="+ca, "GIT_SSH_COMMAND=false", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(root, "empty-config"))
	execute := func(extra []string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin, append([]string{"-test.run=^TestGitCLIChild$", "--", "--config", filepath.Join(root, "config")}, args...)...)
		cmd.Env = append(append([]string{}, childEnv...), extra...)
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				code = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if strings.Contains(string(out), token) || strings.Contains(string(out), base64.StdEncoding.EncodeToString([]byte(token))) {
			t.Fatal("token leaked in output")
		}
		return code, string(out)
	}
	raw := "ssh://git@127.0.0.1:12345/TEAM/repo.git"
	for _, args := range [][]string{{"git", "--dry-run", "--", "clone", raw, filepath.Join(root, "preview")}, {"git", "--", "ls-remote", "--heads", raw}, {"git", "--", "clone", "--origin", "upstream", raw, filepath.Join(root, "checkout")}, {"git", "--", "-C", filepath.Join(root, "checkout"), "fetch", "upstream"}} {
		if code, out := execute(nil, args...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "preview")); !os.IsNotExist(err) {
		t.Fatal("dry run cloned")
	}
	if code, out := execute([]string{"BITBUCKET_CLI_READ_ONLY=1", "BITBUCKET_TOKEN="}, "git", "--fallback", "--", "fetch", raw); code != 5 || !strings.Contains(out, "READONLY_BLOCKED") {
		t.Fatalf("read-only: %d %s", code, out)
	}
	if code, out := execute([]string{"BITBUCKET_CLI_READ_ONLY=1"}, "git", "--fallback", "--", "clone", repoAuthor, filepath.Join(root, "local-checkout")); code != 0 {
		t.Fatalf("unrelated Git inherited Bitbucket read-only: %d %s", code, out)
	}
	if code, out := execute([]string{"BITBUCKET_TOKEN=rejected"}, "git", "--fallback", "--", "ls-remote", raw); code != 4 {
		t.Fatalf("rejected credential: %d %s", code, out)
	}
	if code, out := execute(nil, "git", "--", "clone", raw, filepath.Join(root, "checkout")); code != 128 {
		t.Fatalf("Git status was lost: %d %s", code, out)
	}

	// The bridge must also resolve the native store without a token environment.
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	stored, _ := json.Marshal(map[string]string{auth.AccountKey(base, "pat"): token})
	if err := os.WriteFile(filepath.Join(configDir, "credentials"), stored, 0600); err != nil {
		t.Fatal(err)
	}
	if code, out := execute([]string{"BITBUCKET_TOKEN="}, "git", "--", "ls-remote", raw); code != 0 {
		t.Fatalf("native store: %d %s", code, out)
	}

	t.Run("fx init and sync", func(t *testing.T) {
		entry := os.Getenv("FX_GIT_TEST_ENTRY")
		if entry == "" {
			t.Skip("set FX_GIT_TEST_ENTRY to a built fx bin/fx.js for cross-repository acceptance")
		}
		shimDir := filepath.Join(root, "bin")
		_ = os.MkdirAll(shimDir, 0700)
		script := "#!/bin/sh\nexec '" + strings.ReplaceAll(bin, "'", "'\\''") + "' -test.run='^TestGitCLIChild$' -- \"$@\"\n"
		_ = os.WriteFile(filepath.Join(shimDir, "bitbucket-cli"), []byte(script), 0700)
		manifestAuthor := makeRepo("manifest", map[string]string{"manifest/workspace.yaml": "schema_version: 2\nproviders:\n  code-host: { provider: bitbucket-cli, vars: { host: 'ssh://git@127.0.0.1:12345' } }\ndefaults: { upstream_project: TEAM, branch: main }\nrepos:\n  - { name: repo, desc: Example }\n", "AGENTS.md": "# Example workspace\n"})
		fxHome := filepath.Join(root, "fx-home")
		_ = os.MkdirAll(fxHome, 0700)
		_ = os.WriteFile(filepath.Join(fxHome, "config.yaml"), []byte("agents: []\n"), 0600)
		workspace := filepath.Join(root, "workspace")
		fx := func(args ...string) string {
			t.Helper()
			cmd := exec.Command("node", append([]string{entry}, args...)...)
			cmd.Env = append(append([]string{}, childEnv...), "PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"), "FX_HOME="+fxHome, "FX_UPDATE_CHECK=0")
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				t.Fatalf("fx %v: %v\n%s\n%s", args, err, out.String(), stderr.String())
			}
			if strings.Contains(out.String()+stderr.String(), token) {
				t.Fatal("fx exposed token")
			}
			return out.String()
		}
		fx("init", workspace, "--from", "ssh://git@127.0.0.1:12345/TEAM/manifest.git", "--format", "json", "--no-input")
		if _, err := os.Stat(filepath.Join(workspace, "repo", "README")); err != nil {
			t.Fatal("business repository was not cloned", err)
		}
		// Advance both remotes so the second invocation proves authenticated
		// refresh and fast-forward, not just another successful no-op.
		for _, item := range []struct{ dir, name, file string }{{repoAuthor, "repo", "README"}, {manifestAuthor, "manifest", "AGENTS.md"}} {
			if err := os.WriteFile(filepath.Join(item.dir, item.file), []byte("updated\n"), 0600); err != nil {
				t.Fatal(err)
			}
			runGit("-C", item.dir, "add", item.file)
			runGit("-C", item.dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "update")
			runGit("-C", item.dir, "push", filepath.Join(host, "scm", "TEAM", item.name+".git"), "main")
		}
		fx("--workspace", workspace, "sync", "--only", "repos", "--format", "json", "--no-input")
		content, err := os.ReadFile(filepath.Join(workspace, "repo", "README"))
		if err != nil || string(content) != "updated\n" {
			t.Fatal("fx did not fast-forward the business checkout", err)
		}
		if got := runGit("-C", filepath.Join(workspace, "repo"), "config", "--get", "remote.upstream.url"); got != raw {
			t.Fatalf("unexpected remote %s", got)
		}
	})
}

func TestGitAuthenticationFlavors(t *testing.T) {
	for _, tt := range []struct {
		credential auth.Credential
		cloud      bool
		want       string
	}{
		{auth.Credential{Scheme: "pat", Secret: "token"}, false, "Bearer token"},
		{auth.Credential{Scheme: "basic", Username: "alice", Secret: "password"}, false, "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:password"))},
		{auth.Credential{Scheme: "basic", Username: "alice@example.com", Secret: "api-token"}, true, "Basic " + base64.StdEncoding.EncodeToString([]byte("x-bitbucket-api-token-auth:api-token"))},
		{auth.Credential{Scheme: "pat", Secret: "access-token"}, true, "Basic " + base64.StdEncoding.EncodeToString([]byte("x-token-auth:access-token"))},
	} {
		if got := gitHeader(tt.credential, tt.cloud); got != tt.want {
			t.Errorf("cloud=%v scheme=%s: wrong Git identity", tt.cloud, tt.credential.Scheme)
		}
	}
}
