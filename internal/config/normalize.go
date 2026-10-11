package config

import kit "github.com/angelmsger/agent-cli-kit/config"

// NormalizeContextName trims and lower-cases a context name about to be saved.
// Never apply it to a name being looked up: File.Context already ignores case,
// so legacy mixed-case configs keep working until they are next re-saved.
func NormalizeContextName(s string) string { return kit.NormalizeContextName(s) }

// NormalizeUsername lowercases an *email* username and otherwise preserves the
// caller's casing. Atlassian Cloud authenticates by email + API token and
// treats the address case-insensitively, while Data Center / Server usernames
// may map to case-sensitive LDAP / AD identifiers. Both forms are trimmed.
func NormalizeUsername(s string) string { return kit.NormalizeUsername(s) }
