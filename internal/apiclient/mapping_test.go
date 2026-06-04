package apiclient

import "testing"

// TestMapDCCommentResolution covers the Data Center comment resolution/task
// signals: state == "RESOLVED" -> Resolved, severity == "BLOCKER" -> Task.
func TestMapDCCommentResolution(t *testing.T) {
	cases := []struct {
		name         string
		state        string
		severity     string
		wantResolved bool
		wantTask     bool
	}{
		{"open normal", "OPEN", "NORMAL", false, false},
		{"resolved", "RESOLVED", "NORMAL", true, false},
		{"open task", "OPEN", "BLOCKER", false, true},
		{"resolved task", "RESOLVED", "BLOCKER", true, true},
		{"empty defaults", "", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapDCComment(1, dcComment{ID: 5, Text: "x", State: tc.state, Severity: tc.severity})
			if got.Resolved != tc.wantResolved {
				t.Errorf("Resolved = %v, want %v", got.Resolved, tc.wantResolved)
			}
			if got.Task != tc.wantTask {
				t.Errorf("Task = %v, want %v", got.Task, tc.wantTask)
			}
		})
	}
}

// TestMapCloudCommentResolution covers the Cloud signal: a non-nil `resolution`
// object marks the comment's thread resolved.
func TestMapCloudCommentResolution(t *testing.T) {
	open := cloudComment{ID: 1}
	if got := mapCloudComment(1, open); got.Resolved {
		t.Errorf("open comment: Resolved = true, want false")
	}

	resolved := cloudComment{ID: 2}
	resolved.Resolution = &struct {
		Type string `json:"type"`
	}{Type: "resolved"}
	if got := mapCloudComment(1, resolved); !got.Resolved {
		t.Errorf("resolved comment: Resolved = false, want true")
	}
	// Cloud tasks live on a separate endpoint; Task stays false here.
	if got := mapCloudComment(1, resolved); got.Task {
		t.Errorf("cloud comment Task = true, want false")
	}
}
