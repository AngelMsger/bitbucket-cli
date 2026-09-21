package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/angelmsger/bitbucket-cli/pkg/transport"
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

// Pin each backend's decline contract and prove the preview matches the live
// request without sending a write, including under local read-only posture.
func TestDeclinePRPayloadParity(t *testing.T) {
	for _, flavor := range []Flavor{FlavorCloud, FlavorDataCenter} {
		for _, message := range []string{"", "Superseded by the replacement PR"} {
			t.Run(string(flavor)+"/"+message, func(t *testing.T) {
				prPath := "/2.0/repositories/PROJ/repo/pullrequests/7"
				wantBody := map[string]any{"message": message}
				wantReads := 0
				if flavor == FlavorDataCenter {
					prPath = "/rest/api/1.0/projects/PROJ/repos/repo/pull-requests/7"
					wantBody = map[string]any{"version": float64(9)}
					if message != "" {
						wantBody["comment"] = message
					}
					wantReads = 1
				}
				wantURI := prPath + "/decline"
				if flavor == FlavorDataCenter {
					wantURI += "?version=9"
				}
				reads, writes := 0, 0
				var sentBody map[string]any
				c := newWriteTestClient(t, flavor, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch r.Method {
					case http.MethodGet:
						reads++
						if flavor != FlavorDataCenter || r.URL.RequestURI() != prPath {
							t.Errorf("unexpected version read: %s", r.URL.RequestURI())
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "version": 9})
					case http.MethodPost:
						writes++
						if r.URL.RequestURI() != wantURI {
							t.Errorf("POST URI = %q, want %q", r.URL.RequestURI(), wantURI)
						}
						if err := json.NewDecoder(r.Body).Decode(&sentBody); err != nil {
							t.Errorf("decode decline body: %v", err)
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "state": "DECLINED"})
					default:
						t.Errorf("unexpected method: %s", r.Method)
						w.WriteHeader(http.StatusMethodNotAllowed)
					}
				}))
				req := DeclinePRReq{Repo: RepoRef{Workspace: "PROJ", Slug: "repo"}, ID: 7, Message: message}
				plan, err := NewReadOnly(c).DescribeWrite(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if reads != wantReads || writes != 0 {
					t.Fatalf("preview requests = %d reads, %d writes; want %d reads, no writes", reads, writes, wantReads)
				}
				if plan.Method != http.MethodPost || plan.URL != c.BaseURL()+wantURI {
					t.Fatalf("preview target = %s %s", plan.Method, plan.URL)
				}
				raw, err := json.Marshal(plan.Payload)
				if err != nil {
					t.Fatal(err)
				}
				var plannedBody map[string]any
				if err := json.Unmarshal(raw, &plannedBody); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(plannedBody, wantBody) {
					t.Fatalf("preview body = %v, want %v", plannedBody, wantBody)
				}
				pr, err := c.DeclinePR(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if pr.ID != 7 || pr.State != "DECLINED" || reads != 2*wantReads || writes != 1 {
					t.Fatalf("decline result = %+v; requests = %d reads, %d writes", pr, reads, writes)
				}
				if !reflect.DeepEqual(sentBody, plannedBody) {
					t.Errorf("live body = %v, preview = %v", sentBody, plannedBody)
				}
			})
		}
	}
}

func TestForkImplicitTargetMatchesRegistry(t *testing.T) {
	req := ForkRepoReq{Source: RepoRef{Workspace: "source", Slug: "repo"}}
	for _, f := range []Flavor{FlavorCloud, FlavorDataCenter} {
		sup := capabilitySupportFor(CapRepoForkImplicitTarget, f)
		_, err := offlineClient(f).DescribeWrite(context.Background(), req)
		if sup.Supported() && err != nil {
			t.Errorf("flavor %q supports an implicit fork target but validation failed: %v", f, err)
		}
		if !sup.Supported() && err == nil {
			t.Errorf("flavor %q rejects an implicit fork target but validation allowed it", f)
		}
	}
}

func TestForkRepositoryPayloadGolden(t *testing.T) {
	req := ForkRepoReq{
		Source:    RepoRef{Workspace: "source", Slug: "repo"},
		Workspace: "target",
		Name:      "repo-fork",
	}

	cloud := describePayloadJSON(t, offlineClient(FlavorCloud), req)
	wantCloud := `{
  "name": "repo-fork",
  "workspace": {
    "slug": "target"
  }
}`
	if cloud != wantCloud {
		t.Errorf("Cloud fork payload drift:\n got:\n%s\nwant:\n%s", cloud, wantCloud)
	}

	dc := describePayloadJSON(t, offlineClient(FlavorDataCenter), req)
	wantDC := `{
  "name": "repo-fork",
  "project": {
    "key": "target"
  }
}`
	if dc != wantDC {
		t.Errorf("DC fork payload drift:\n got:\n%s\nwant:\n%s", dc, wantDC)
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
