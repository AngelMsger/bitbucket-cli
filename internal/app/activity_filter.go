package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
)

type activityWindow struct {
	from time.Time
	to   time.Time
	set  bool
}

func resolveActivityWindow(since, from, to string, now time.Time) (activityWindow, error) {
	if since != "" && (from != "" || to != "") {
		return activityWindow{}, fmt.Errorf("--since cannot be combined with --from or --to")
	}
	if to != "" && from == "" {
		return activityWindow{}, fmt.Errorf("--to requires --from")
	}
	if since == "" && from == "" {
		return activityWindow{}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	var start, end time.Time
	var err error
	if since != "" {
		var duration time.Duration
		duration, err = parseActivityDuration(since)
		if err != nil || duration <= 0 {
			return activityWindow{}, fmt.Errorf("invalid --since %q: use a positive duration such as 24h or 7d", since)
		}
		start, end = now.Add(-duration), now
	} else {
		start, err = parseActivityInstant(from)
		if err != nil {
			return activityWindow{}, fmt.Errorf("invalid --from %q: %w", from, err)
		}
		end = now
		if to != "" {
			end, err = parseActivityInstant(to)
			if err != nil {
				return activityWindow{}, fmt.Errorf("invalid --to %q: %w", to, err)
			}
		}
	}
	if !end.After(start) {
		return activityWindow{}, fmt.Errorf("--to must be later than --from")
	}
	return activityWindow{from: start, to: end, set: true}, nil
}

func parseActivityDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if len(value) > 1 {
		n, err := strconv.Atoi(value[:len(value)-1])
		if err == nil {
			switch value[len(value)-1] {
			case 'd':
				return time.Duration(n) * 24 * time.Hour, nil
			case 'w':
				return time.Duration(n) * 7 * 24 * time.Hour, nil
			}
		}
	}
	return time.ParseDuration(value)
}

func parseClosedSince(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	duration, err := parseActivityDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("use a positive duration such as 24h or 7d")
	}
	seconds := int64(duration / time.Second)
	if seconds == 0 {
		return 0, fmt.Errorf("duration must be at least one second")
	}
	return seconds, nil
}

func parseActivityInstant(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, time.UTC); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("use RFC3339 or a date in YYYY-MM-DD form (date-only values are UTC)")
}

// activityInstant parses Cloud RFC3339 or Data Center epoch milliseconds.
func activityInstant(value string) (time.Time, error) {
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.UnixMilli(millis), nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func normalizeActivityKinds(values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return nil, nil
	}
	allowed := map[string]string{
		"comment": "comment", "commented": "comment",
		"approval": "approval", "approve": "approval", "approved": "approval",
		"decline": "decline", "declined": "decline",
		"merge": "merge", "merged": "merge",
		"update": "update",
	}
	out := make(map[string]struct{})
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			canonical, ok := allowed[part]
			if !ok {
				return nil, fmt.Errorf("unknown activity kind %q (use comment, approval, decline, merge, or update)", part)
			}
			out[canonical] = struct{}{}
		}
	}
	return out, nil
}

func filterActivities(items []apiclient.Activity, window activityWindow, actor *apiclient.User, kinds map[string]struct{}) ([]apiclient.Activity, error) {
	out := make([]apiclient.Activity, 0, len(items))
	for _, item := range items {
		if len(kinds) > 0 {
			if _, ok := kinds[canonicalActivityKind(item)]; !ok {
				continue
			}
		}
		if actor != nil && !sameActivityUser(*actor, item.Actor) {
			continue
		}
		if window.set {
			when, err := activityInstant(item.When)
			if err != nil {
				return nil, fmt.Errorf("activity has an invalid timestamp %q: %w", item.When, err)
			}
			if when.Before(window.from) || !when.Before(window.to) {
				continue
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func canonicalActivityKind(item apiclient.Activity) string {
	kind := strings.ToLower(item.Kind)
	if kind == "approved" {
		return "approval"
	}
	if kind == "update" {
		switch strings.ToUpper(item.State) {
		case "MERGED":
			return "merge"
		case "DECLINED":
			return "decline"
		}
	}
	return kind
}

func sameActivityUser(left, right apiclient.User) bool {
	leftIDs := activityUserIDs(left)
	for id := range activityUserIDs(right) {
		if _, ok := leftIDs[id]; ok {
			return true
		}
	}
	return false
}

func activityUserIDs(user apiclient.User) map[string]struct{} {
	out := make(map[string]struct{})
	for _, value := range []string{user.AccountID, user.UUID, user.Name, user.Slug} {
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "{}"))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func sortActivitiesNewestFirst(items []apiclient.Activity) {
	sort.SliceStable(items, func(i, j int) bool {
		left, leftErr := activityInstant(items[i].When)
		right, rightErr := activityInstant(items[j].When)
		if leftErr != nil || rightErr != nil {
			return items[i].When > items[j].When
		}
		return left.After(right)
	})
}

func uniqueActivityRefs(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
