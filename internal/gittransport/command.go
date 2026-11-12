// Package gittransport runs a bounded set of Git network operations using native
// Bitbucket credentials. Repository URLs and local Git configuration stay public.
package gittransport

import (
	"fmt"
	"os/exec"
	"strings"
)

// Command keeps Git's original arguments, including checkout and refspec choices.
type Command struct {
	Args      []string
	Prefix    []string
	Operation string
	Target    string
}

// Parse accepts one explicitly named remote. Multi-remote operations and external
// upload/receive programs cannot be authenticated as a single repository.
func Parse(args []string) (Command, error) {
	c := Command{Args: args}
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-C", "-c":
			if i+1 >= len(args) {
				return c, fmt.Errorf("missing Git option value")
			}
			if args[i] == "-c" {
				key, _, _ := strings.Cut(args[i+1], "=")
				if !strings.EqualFold(key, "core.quotepath") && !strings.EqualFold(key, "protocol.version") {
					return c, fmt.Errorf("only core.quotepath and protocol.version Git -c settings are supported")
				}
			}
			c.Prefix = append(c.Prefix, args[i:i+2]...)
			i += 2
		case "--no-optional-locks":
			c.Prefix = append(c.Prefix, args[i])
			i++
		default:
			c.Operation = args[i]
			i++
			goto operation
		}
	}
operation:
	if c.Operation != "clone" && c.Operation != "fetch" && c.Operation != "ls-remote" && c.Operation != "push" {
		return c, fmt.Errorf("supported Git operations: clone, fetch, ls-remote, push")
	}
	values := map[string]bool{
		"--branch":          true,
		"-b":                true,
		"--origin":          true,
		"-o":                true,
		"--depth":           true,
		"--shallow-since":   true,
		"--shallow-exclude": true,
		"--filter":          true,
		"--jobs":            true,
		"-j":                true,
	}
	booleans := map[string]bool{
		"--quiet":                 true,
		"-q":                      true,
		"--verbose":               true,
		"-v":                      true,
		"--prune":                 true,
		"--tags":                  true,
		"--no-tags":               true,
		"--heads":                 true,
		"--branches":              true,
		"--refs":                  true,
		"--symref":                true,
		"--exit-code":             true,
		"--get-url":               true,
		"--porcelain":             true,
		"--no-follow-tags":        true,
		"--single-branch":         true,
		"--no-single-branch":      true,
		"--bare":                  true,
		"--no-checkout":           true,
		"--no-recurse-submodules": true,
		"--recurse-submodules=no": true,
		"--progress":              true,
		"--no-progress":           true,
		"--dry-run":               true,
		"--force-with-lease":      true,
		"--force":                 true,
		"--atomic":                true,
	}
	positionalOnly := false
	for i < len(args) {
		word := args[i]
		i++
		if word == "--" && !positionalOnly {
			positionalOnly = true
			continue
		}
		if positionalOnly || !strings.HasPrefix(word, "-") {
			if c.Target == "" {
				c.Target = word
			}
			continue
		}
		key, _, equals := strings.Cut(word, "=")
		if values[key] {
			if !equals {
				if i >= len(args) {
					return c, fmt.Errorf("missing Git option value")
				}
				i++
			}
			continue
		}
		if !booleans[word] {
			return c, fmt.Errorf("unsupported Git option %s; name one remote and disable recursive submodules", key)
		}
	}
	if c.Target == "" {
		return c, fmt.Errorf("name the repository URL or remote explicitly")
	}
	return c, nil
}

// URL reads raw remote configuration: URL rewrites are scoped by Run, not persisted.
func (c Command) URL() (string, error) {
	if c.Operation == "clone" || strings.Contains(c.Target, ":") || strings.Contains(c.Target, "/") {
		return c.Target, nil
	}
	key := "remote." + c.Target + ".url"
	if c.Operation == "push" {
		out, err := exec.Command("git", append(append([]string{}, c.Prefix...), "config", "--get-all", "remote."+c.Target+".pushurl")...).Output()
		if err == nil {
			return singleURL(string(out))
		}
	}
	out, err := exec.Command("git", append(append([]string{}, c.Prefix...), "config", "--get-all", key)...).Output()
	if err != nil {
		return "", fmt.Errorf("cannot read the named Git remote")
	}
	return singleURL(string(out))
}

func singleURL(value string) (string, error) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) != 1 || lines[0] == "" {
		return "", fmt.Errorf("the Git remote must have exactly one URL")
	}
	return lines[0], nil
}
