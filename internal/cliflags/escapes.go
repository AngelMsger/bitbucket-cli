package cliflags

import "strings"

// BodyFlags names the long flags whose values are free-text Markdown bodies a
// human or agent types into the shell. When such a value carries a literal
// backslash-n, the shell almost never expanded it (double quotes don't), so the
// user meant a real newline — and Bitbucket would otherwise render the literal
// "\n". InterpretEscapes decodes a small escape whitelist in exactly these
// values. The matching `*-file` / stdin inputs (e.g. --content-file) are the
// exact-bytes escape hatch and are deliberately absent here.
var BodyFlags = map[string]bool{
	"content":     true, // comment add / comment update
	"description": true, // pr create / pr update / repo
	"message":     true, // pr decline / pr merge
	"title":       true, // pr create / pr update
}

// interpretEscapes decodes \n, \r, \t and \\ in a body value. It is the inverse
// of the slip it fixes: the shell handed over a literal backslash-n the user
// meant as a newline. \\ is honored so a literal backslash sequence is still
// expressible (\\n -> \n); any other \x — a regex \d, a Windows path — is left
// verbatim, and a trailing lone backslash is kept as-is. The returned detail
// lists the distinct escapes decoded, for the correction notice.
func interpretEscapes(s string) (out, detail string, changed bool) {
	if !strings.ContainsRune(s, '\\') {
		return s, "", false
	}
	var b strings.Builder
	b.Grow(len(s))
	var seen []string
	mark := func(label string) {
		for _, l := range seen {
			if l == label {
				return
			}
		}
		seen = append(seen, label)
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				mark(`\n→newline`)
				i++
				continue
			case 'r':
				b.WriteByte('\r')
				mark(`\r→carriage-return`)
				i++
				continue
			case 't':
				b.WriteByte('\t')
				mark(`\t→tab`)
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				mark(`\\→\`)
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	if len(seen) == 0 {
		return s, "", false
	}
	return b.String(), strings.Join(seen, ", "), true
}

// InterpretEscapes decodes the escape whitelist (see interpretEscapes) in the
// values of BodyFlags found in args, returning the rewritten argv and one
// "escape" Correction per flag whose value changed. It mirrors Normalize's argv
// walk: tokens after `--`, and shell-completion invocations, are left untouched.
// Run it after Normalize so canonicalized flag names are already in place.
func InterpretEscapes(args []string, bodyFlags map[string]bool) (out []string, corrections []Correction) {
	if len(args) > 0 && strings.HasPrefix(args[0], "__complete") {
		return args, nil
	}
	out = make([]string, 0, len(args))
	endFlags := false
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if endFlags || !strings.HasPrefix(tok, "--") || tok == "--" {
			if tok == "--" {
				endFlags = true
			}
			out = append(out, tok)
			continue
		}
		name, val, hasEq := splitEq(tok[2:])
		if !bodyFlags[name] {
			out = append(out, tok)
			continue
		}
		if hasEq {
			decoded, detail, changed := interpretEscapes(val)
			if changed {
				corrections = append(corrections, Correction{Flag: "--" + name, Detail: detail, Kind: "escape"})
				out = append(out, "--"+name+"="+decoded)
			} else {
				out = append(out, tok)
			}
			continue
		}
		// Space form: the value is the next token (cobra consumes it the same
		// way, even when it looks like a flag), so decode that.
		out = append(out, tok)
		if i+1 < len(args) {
			decoded, detail, changed := interpretEscapes(args[i+1])
			if changed {
				corrections = append(corrections, Correction{Flag: "--" + name, Detail: detail, Kind: "escape"})
				out = append(out, decoded)
			} else {
				out = append(out, args[i+1])
			}
			i++
		}
	}
	return out, corrections
}
