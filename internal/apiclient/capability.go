package apiclient

// This file is the single source of truth for features whose behaviour differs
// between Bitbucket Cloud and Data Center. Bitbucket exposes the same concepts
// through different APIs, and the recurring bug class in this CLI is a Cloud
// branch honouring an input that the DC branch silently drops (or vice versa).
//
// Anything NOT listed here is assumed uniformly supported on both flavors. List
// a capability only when the flavors diverge, then drive the runtime guard, the
// help/docs, and the parity test from this one table so they cannot drift.

// Capability names a flavor-sensitive feature.
type Capability string

const (
	// CapPRRequestChanges: the "request changes" / "needs work" review verdict.
	CapPRRequestChanges Capability = "pr.request-changes"
	// CapPRCloseSourceBranch: deleting the source branch as part of a merge.
	CapPRCloseSourceBranch Capability = "pr.close-source-branch"
	// CapPRCrossForkCreate: opening a PR from a fork into an upstream repo.
	CapPRCrossForkCreate Capability = "pr.cross-fork-create"
)

// SupportLevel describes how a flavor provides a capability.
type SupportLevel string

const (
	// SupportNative: the API does it directly (a single request / native field).
	SupportNative SupportLevel = "native"
	// SupportEmulated: the CLI reproduces the behaviour with extra calls.
	SupportEmulated SupportLevel = "emulated"
	// SupportUnsupported: not available on this flavor yet.
	SupportUnsupported SupportLevel = "unsupported"
)

// Support is a flavor's support for a capability, with a reason for the
// non-native cases (shown in errors / help).
type Support struct {
	Level  SupportLevel `json:"level"`
	Reason string       `json:"reason,omitempty"`
}

// Supported reports whether the capability can be used at all on this flavor.
func (s Support) Supported() bool { return s.Level != SupportUnsupported }

// capabilitySupport is the divergence table. A flavor missing from a row (or a
// capability missing entirely) defaults to native support.
var capabilitySupport = map[Capability]map[Flavor]Support{
	CapPRRequestChanges: {
		FlavorCloud: {Level: SupportNative},
		FlavorDataCenter: {Level: SupportUnsupported,
			Reason: "DC models this as a participant-status (NEEDS_WORK) PUT keyed on the caller's user slug, which the DC client cannot resolve yet (no working whoami)"},
	},
	CapPRCloseSourceBranch: {
		FlavorCloud: {Level: SupportNative},
		FlavorDataCenter: {Level: SupportEmulated,
			Reason: "DC has no native flag; the source branch is deleted with a follow-up call after the merge. Merge-time only — DC cannot pre-set it at PR creation (`pr create --close-source-branch` is rejected there)"},
	},
	CapPRCrossForkCreate: {
		FlavorCloud: {Level: SupportNative},
		FlavorDataCenter: {Level: SupportNative,
			Reason: "DC also requires an explicit --target (the upstream destination branch)"},
	},
}

// supportFor returns this client's flavor's support for a capability.
func (c *apiClient) supportFor(cap Capability) Support {
	return capabilitySupportFor(cap, c.flavor)
}

func capabilitySupportFor(cap Capability, flavor Flavor) Support {
	if byFlavor, ok := capabilitySupport[cap]; ok {
		if s, ok := byFlavor[flavor]; ok {
			return s
		}
	}
	return Support{Level: SupportNative}
}

// CapabilityMatrix returns a copy of the full divergence table, for rendering in
// help / docs. Keys are stable capability identifiers.
func CapabilityMatrix() map[Capability]map[Flavor]Support {
	out := make(map[Capability]map[Flavor]Support, len(capabilitySupport))
	for cap, byFlavor := range capabilitySupport {
		row := make(map[Flavor]Support, len(byFlavor))
		for f, s := range byFlavor {
			row[f] = s
		}
		out[cap] = row
	}
	return out
}
