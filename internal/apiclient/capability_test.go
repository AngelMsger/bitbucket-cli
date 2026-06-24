package apiclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/internal/transport"
)

// offlineClient builds a client with a fixed base URL and no server. It is only
// valid for DescribeWrite on operations whose builder makes no HTTP call
// (e.g. CreatePR); anything that pre-fetches a version will panic on dial.
func offlineClient(flavor Flavor) Client {
	return New(Config{Flavor: flavor, BaseURL: "https://bb.example", Transport: transport.New(transport.Options{})})
}

// TestCapabilityMatrixComplete is the guard that keeps the divergence table
// honest: every listed capability must spell out BOTH flavors, and any
// non-native support must carry a reason (surfaced in errors / help). A new
// capability added without covering both flavors fails here.
func TestCapabilityMatrixComplete(t *testing.T) {
	for cap, byFlavor := range CapabilityMatrix() {
		for _, f := range []Flavor{FlavorCloud, FlavorDataCenter} {
			s, ok := byFlavor[f]
			if !ok {
				t.Errorf("capability %q missing an entry for flavor %q", cap, f)
				continue
			}
			if s.Level != SupportNative && strings.TrimSpace(s.Reason) == "" {
				t.Errorf("capability %q on %q is %q but has no reason", cap, f, s.Level)
			}
		}
	}
}

// TestRequestChangesMatchesRegistry pins the runtime guard to the registry: the
// command must be rejected on a flavor the table marks unsupported, and allowed
// where it is supported. This is the consistency check that would have caught
// the old hard-coded "Cloud-only" guard drifting from reality.
func TestRequestChangesMatchesRegistry(t *testing.T) {
	for _, f := range []Flavor{FlavorCloud, FlavorDataCenter} {
		sup := capabilitySupportFor(CapPRRequestChanges, f)
		// DescribeWrite exercises the same guard without sending HTTP.
		_, err := offlineClient(f).DescribeWrite(context.Background(), RequestChangesReq{
			Repo: RepoRef{Workspace: "ws", Slug: "repo"}, ID: 1, Request: true,
		})
		if sup.Supported() && err != nil {
			t.Errorf("flavor %q marks request-changes supported but the guard errored: %v", f, err)
		}
		if !sup.Supported() && err == nil {
			t.Errorf("flavor %q marks request-changes unsupported but the guard allowed it", f)
		}
	}
}

// describePayloadJSON renders a write op's planned payload as canonical JSON
// (map keys sorted by encoding/json) for golden comparison.
func describePayloadJSON(t *testing.T, c Client, op any) string {
	t.Helper()
	plan, err := c.DescribeWrite(context.Background(), op)
	if err != nil {
		t.Fatalf("DescribeWrite: %v", err)
	}
	b, err := json.MarshalIndent(plan.Payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

// TestCreatePRPayloadGolden snapshots the exact create-PR wire payload for each
// flavor. The point is review-time visibility: if someone wires a new field on
// one flavor and forgets the other, the golden for that flavor changes and the
// other does not — the asymmetry is staring at you in the diff.
func TestCreatePRPayloadGolden(t *testing.T) {
	req := CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t", Description: "d",
		Source: "feature", Destination: "dev",
	}

	cloud := describePayloadJSON(t, offlineClient(FlavorCloud), req)
	wantCloud := `{
  "description": "d",
  "destination": {
    "branch": {
      "name": "dev"
    }
  },
  "source": {
    "branch": {
      "name": "feature"
    }
  },
  "title": "t"
}`
	if cloud != wantCloud {
		t.Errorf("Cloud create payload drift:\n got:\n%s\nwant:\n%s", cloud, wantCloud)
	}

	dc := describePayloadJSON(t, offlineClient(FlavorDataCenter), req)
	wantDC := `{
  "closed": false,
  "description": "d",
  "fromRef": {
    "id": "refs/heads/feature",
    "repository": {
      "project": {
        "key": "UP"
      },
      "slug": "repo"
    }
  },
  "open": true,
  "state": "OPEN",
  "title": "t",
  "toRef": {
    "id": "refs/heads/dev",
    "repository": {
      "project": {
        "key": "UP"
      },
      "slug": "repo"
    }
  }
}`
	if dc != wantDC {
		t.Errorf("DC create payload drift:\n got:\n%s\nwant:\n%s", dc, wantDC)
	}
}

// TestCreatePRCloseSourceBranchParity exercises every flavor with the
// --close-source-branch intent set, the field that previously slipped through
// the DC create branch. Cloud sends it natively; DC create must reject it (the
// emulation only exists at merge time) rather than silently drop it.
func TestCreatePRCloseSourceBranchParity(t *testing.T) {
	req := CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t",
		Source: "feature", Destination: "dev", CloseSourceBranch: true,
	}

	cloud := describePayloadJSON(t, offlineClient(FlavorCloud), req)
	if !strings.Contains(cloud, `"close_source_branch": true`) {
		t.Errorf("Cloud create should carry close_source_branch; got:\n%s", cloud)
	}

	_, err := offlineClient(FlavorDataCenter).DescribeWrite(context.Background(), req)
	if err == nil {
		t.Error("DC create with --close-source-branch should be rejected, not silently dropped")
	}
}

// TestCreatePRForkPayloadGolden locks the cross-fork wire shape: fromRef points
// at the fork, toRef at the upstream.
func TestCreatePRForkPayloadGolden(t *testing.T) {
	req := CreatePRReq{
		Repo: RepoRef{Workspace: "UP", Slug: "repo"}, Title: "t",
		Source: "feature", SourceRepo: "FORK/repo", Destination: "dev",
	}
	dc := describePayloadJSON(t, offlineClient(FlavorDataCenter), req)
	if !strings.Contains(dc, `"key": "FORK"`) {
		t.Errorf("fork fromRef should reference project FORK; got:\n%s", dc)
	}
	// fromRef → fork, toRef → upstream.
	from := strings.Index(dc, `"fromRef"`)
	to := strings.Index(dc, `"toRef"`)
	if from < 0 || to < 0 || strings.Index(dc, `"FORK"`) > to {
		t.Errorf("expected fromRef(FORK) before toRef(UP); got:\n%s", dc)
	}
}
