// Secrets enter only Git's child environment, scoped to an HTTPS repository.
package gittransport

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Authentication struct{ Original, HTTPS, Header, Secret string }

func Environment(inherited []string, a Authentication) []string {
	env := make([]string, 0, len(inherited)+16)
	for _, entry := range inherited {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_TRACE") || strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") || key == "GIT_CONFIG_COUNT" || key == "GIT_CONFIG_PARAMETERS" || key == "GIT_CURL_VERBOSE" || key == "GIT_TERMINAL_PROMPT" || key == "GIT_ASKPASS" || key == "SSH_ASKPASS" {
			continue
		}
		env = append(env, entry)
	}
	// The empty header clears inherited extra headers. Disabling helpers prevents
	// Git from persisting or substituting another identity after an HTTP challenge.
	entries := [][2]string{
		{"http.extraHeader", ""},
		{"http." + a.HTTPS + ".extraHeader", ""},
		{"http." + a.HTTPS + ".extraHeader", "Authorization: " + a.Header},
		{"http.followRedirects", "false"},
		{"http." + a.HTTPS + ".followRedirects", "false"},
		{"credential.helper", ""},
		{"core.askPass", ""},
		{"url." + a.HTTPS + ".insteadOf", a.Original},
		{"submodule.recurse", "false"},
		{"fetch.recurseSubmodules", "false"},
		{"push.recurseSubmodules", "no"},
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "GIT_TRACE2=0", "GIT_TRACE2_EVENT=0", "GIT_TRACE2_PERF=0", "GIT_CONFIG_COUNT="+strconv.Itoa(len(entries)))
	for i, e := range entries {
		env = append(env, "GIT_CONFIG_KEY_"+strconv.Itoa(i)+"="+e[0], "GIT_CONFIG_VALUE_"+strconv.Itoa(i)+"="+e[1])
	}
	return env
}

// Run preserves Git's exit status and streams its ordinary output. No secret is
// included in argv, Git configuration files, temporary scripts or returned plans.
func Run(ctx context.Context, c Command, a *Authentication, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, "git", c.Args...)
	cmd.Stdin = os.Stdin
	if a == nil {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	} else {
		cmd.Env = Environment(os.Environ(), *a)
		replacer := strings.NewReplacer(a.Header, "[redacted]", strings.TrimPrefix(a.Header, "Basic "), "[redacted]", a.Secret, "[redacted]")
		out := &redactedWriter{to: stdout, replace: replacer}
		errout := &redactedWriter{to: stderr, replace: replacer}
		cmd.Stdout = out
		cmd.Stderr = errout
		defer out.flush()
		defer errout.flush()
	}
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), nil
	}
	return 0, err
}

// HTTP credentials cannot contain line separators. Buffer incomplete lines so a
// token split across writes is still redacted, including server-echoed errors.
type redactedWriter struct {
	to      io.Writer
	replace *strings.Replacer
	pending []byte
}

func (w *redactedWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	if at := bytes.LastIndexAny(w.pending, "\r\n"); at >= 0 {
		if _, err := io.WriteString(w.to, w.replace.Replace(string(w.pending[:at+1]))); err != nil {
			return 0, err
		}
		w.pending = w.pending[at+1:]
	}
	return len(p), nil
}

func (w *redactedWriter) flush() {
	_, _ = io.WriteString(w.to, w.replace.Replace(string(w.pending)))
	w.pending = nil
}
