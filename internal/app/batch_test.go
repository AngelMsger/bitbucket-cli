package app

import (
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

func TestCollectBatchArgs(t *testing.T) {
	t.Run("positional args pass through", func(t *testing.T) {
		got, err := collectBatchArgs([]string{"1", "2", "3"}, strings.NewReader("ignored"))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []string{"1", "2", "3"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("dash reads stdin, trimming blanks", func(t *testing.T) {
		got, err := collectBatchArgs([]string{"-"}, strings.NewReader("  10 \n\n20\n\t30\t\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, []string{"10", "20", "30"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("empty stdin is an error", func(t *testing.T) {
		_, err := collectBatchArgs([]string{"-"}, strings.NewReader("\n  \n"))
		if err == nil || !strings.Contains(err.Error(), "no items") {
			t.Errorf("want NO_ITEMS error, got %v", err)
		}
	})
}

func TestExecuteBatchReadBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	var active, peak int32

	out, failed, last, anyRetryable := executeBatchRead(items, 3, func(item string) (any, error) {
		cur := atomic.AddInt32(&active, 1)
		for {
			prev := atomic.LoadInt32(&peak)
			if cur <= prev || atomic.CompareAndSwapInt32(&peak, prev, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&active, -1)

		if item == "d" {
			return nil, cerrors.New(cerrors.CategoryNotFound, "NOT_FOUND", "missing")
		}
		if item == "g" {
			e := cerrors.New(cerrors.CategoryServer, "TEMP", "transient")
			e.Retryable = true
			return nil, e
		}
		return "ok-" + item, nil
	})

	if peak > 3 {
		t.Fatalf("peak concurrency = %d, want <= 3", peak)
	}
	if failed != 2 || last == nil || last.Code != "TEMP" || !anyRetryable {
		t.Fatalf("failed=%d last=%v anyRetryable=%v", failed, last, anyRetryable)
	}
	if len(out) != len(items) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(items))
	}
	for i, want := range items {
		if out[i].Ref != want {
			t.Errorf("out[%d].Ref = %q, want %q", i, out[i].Ref, want)
		}
		switch want {
		case "d":
			if out[i].OK || out[i].Error == nil || out[i].Error.Code != "NOT_FOUND" {
				t.Errorf("out[%d] = %+v, want NOT_FOUND error", i, out[i])
			}
		case "g":
			if out[i].OK || out[i].Error == nil || out[i].Error.Code != "TEMP" {
				t.Errorf("out[%d] = %+v, want TEMP error", i, out[i])
			}
		default:
			if !out[i].OK || out[i].Result != "ok-"+want {
				t.Errorf("out[%d] = %+v, want ok-%s", i, out[i], want)
			}
		}
	}
}

func TestBatchReadPartialFailureHint(t *testing.T) {
	ce := cerrors.AsCLIError(batchReadPartialFailure(cerrors.CategoryNotFound, 1, 3, false))
	if ce.Code != "BATCH_PARTIAL_FAILURE" || ce.Retryable {
		t.Fatalf("got code=%s retryable=%v", ce.Code, ce.Retryable)
	}
	if !strings.Contains(ce.Hint, "nothing was modified") || strings.Contains(ce.Hint, "were applied") {
		t.Fatalf("unexpected read-batch hint: %q", ce.Hint)
	}
}
