package errors

// defaultGuidance returns the default hint and next-step commands for a
// category. Callers may override these via WithHint / WithNextSteps when more
// specific guidance is available.
func defaultGuidance(cat Category) (hint string, steps []string) {
	switch cat {
	case CategoryUsage:
		return "The command was invoked incorrectly. Check flags and arguments.",
			[]string{"bitbucket-cli <command> --help"}
	case CategoryConfig:
		return "No usable configuration was found or it is invalid.",
			[]string{"bitbucket-cli config init --pretty", "bitbucket-cli config show --explain"}
	case CategoryAuth:
		return "The server rejected the credentials. The token may be expired or wrong.",
			[]string{"bitbucket-cli auth status", "bitbucket-cli config init --pretty"}
	case CategoryPermission:
		return "The credentials are valid but lack permission for this resource.",
			[]string{"Verify the account can access the workspace, repository or PR in a browser."}
	case CategoryNotFound:
		return "The requested workspace, repository, pull request, branch or commit does not exist.",
			[]string{"bitbucket-cli repo list --workspace <name>", "Double-check the <workspace>/<repo>/<id> reference or URL."}
	case CategoryConflict:
		return "The resource changed since it was last read (version conflict).",
			[]string{"Re-fetch the resource to get its current version, then retry."}
	case CategoryRateLimit:
		return "The server is rate limiting requests. Retry after a short wait.",
			[]string{"Wait and retry; reduce --limit or avoid --all for large queries."}
	case CategoryNetwork:
		return "The server could not be reached (DNS, TLS or timeout).",
			[]string{"bitbucket-cli doctor", "Check --base-url and network connectivity."}
	case CategoryServer:
		return "The Bitbucket server returned an internal error.",
			[]string{"Retry later.", "bitbucket-cli doctor"}
	case CategoryParse:
		return "A response could not be parsed or rendered.",
			[]string{"Retry with --format json --verbose to inspect the raw response."}
	default:
		return "An unexpected internal error occurred.",
			[]string{"Retry with --verbose for details."}
	}
}
