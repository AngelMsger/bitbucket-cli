package app

import (
	"bufio"
	"io"
	"strings"
	"sync"

	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
)

// batchReadConcurrency bounds parallel reads in batch read commands (`pr get`,
// `pr status`, `pr threads`) so large stdin pipelines stay fast without
// flooding Bitbucket rate limits.
const batchReadConcurrency = 4

// batchItem is one entry in a batch operation's aggregated result.
type batchItem struct {
	Ref    string               `json:"ref"`
	OK     bool                 `json:"ok"`
	Result any                  `json:"result,omitempty"`
	Error  *cerrors.PayloadBody `json:"error,omitempty"`
}

// collectBatchArgs returns the items a batch command should operate on. A lone
// "-" positional reads newline-separated items from r (stdin); otherwise the
// positional args are the items verbatim. Blank lines and surrounding
// whitespace are ignored, so `... | xargs`-style and here-doc input both work.
func collectBatchArgs(args []string, r io.Reader) ([]string, error) {
	if len(args) == 1 && args[0] == "-" {
		var items []string
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			if s := strings.TrimSpace(sc.Text()); s != "" {
				items = append(items, s)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_STDIN",
				"could not read items from stdin")
		}
		if len(items) == 0 {
			return nil, cerrors.New(cerrors.CategoryUsage, "NO_ITEMS",
				"no items were provided on stdin")
		}
		return items, nil
	}
	return args, nil
}

// runBatch applies do to each item. With a single item it emits the bare result
// (byte-identical to the pre-batch single-object output, so existing callers and
// scripts are unaffected). With several items it emits a {items, has_more}
// aggregate recording ok / result / error per item, runs every item even when
// some fail, and returns a summarizing error so the process exit code still
// reflects a partial failure. do should return the per-item success payload (or
// nil) and an error; it is responsible for honoring --dry-run itself.
func runBatch(s *appState, items []string, dryRun bool, do func(item string) (any, error)) error {
	if len(items) == 1 {
		res, err := do(items[0])
		if err != nil {
			return err
		}
		return s.emit(res)
	}
	out := make([]batchItem, 0, len(items))
	failed := 0
	var last *cerrors.CLIError
	for _, it := range items {
		res, err := do(it)
		if err != nil {
			ce := cerrors.AsCLIError(err)
			body := ce.Payload().Error
			out = append(out, batchItem{Ref: it, OK: false, Error: &body})
			failed++
			last = ce
			continue
		}
		out = append(out, batchItem{Ref: it, OK: true, Result: res})
	}
	if emitErr := s.emit(map[string]any{"items": out, "has_more": false}); emitErr != nil {
		return emitErr
	}
	if failed > 0 {
		return batchPartialFailure(last.Category, failed, len(items), dryRun)
	}
	return nil
}

// runBatchRead executes read-only per-item lookups with bounded concurrency,
// preserving the caller's input order in the emitted {items, has_more} payload.
// With a single item it emits the bare result so single-target output remains
// byte-identical. Any per-item error is recorded without stopping other items,
// and the command exits non-zero when at least one item failed.
func runBatchRead(s *appState, items []string, do func(item string) (any, error)) error {
	if len(items) == 1 {
		res, err := do(items[0])
		if err != nil {
			return err
		}
		return s.emit(res)
	}
	out, failed, last, anyRetryable := executeBatchRead(items, batchReadConcurrency, do)
	if emitErr := s.emit(map[string]any{"items": out, "has_more": false}); emitErr != nil {
		return emitErr
	}
	if failed > 0 {
		return batchReadPartialFailure(last.Category, failed, len(items), anyRetryable)
	}
	return nil
}

func executeBatchRead(items []string, concurrency int, do func(item string) (any, error)) ([]batchItem, int, *cerrors.CLIError, bool) {
	if concurrency < 1 {
		concurrency = 1
	}
	out := make([]batchItem, len(items))
	errs := make([]*cerrors.CLIError, len(items))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func(idx int, ref string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := do(ref)
			if err != nil {
				ce := cerrors.AsCLIError(err)
				body := ce.Payload().Error
				out[idx] = batchItem{Ref: ref, OK: false, Error: &body}
				errs[idx] = ce
				return
			}
			out[idx] = batchItem{Ref: ref, OK: true, Result: res}
		}(i, it)
	}
	wg.Wait()

	failed := 0
	var last *cerrors.CLIError
	anyRetryable := false
	for _, ce := range errs {
		if ce != nil {
			failed++
			last = ce
			if ce.Retryable {
				anyRetryable = true
			}
		}
	}
	return out, failed, last, anyRetryable
}

// batchReadPartialFailure summarizes a read-only batch with failed items. Unlike
// write batches, nothing was mutated, so retrying the failed items is safe.
func batchReadPartialFailure(cat cerrors.Category, failed, total int, retryable bool) error {
	e := cerrors.Newf(cat, "BATCH_PARTIAL_FAILURE",
		"%d of %d read operations failed; see per-item results on stdout", failed, total).
		WithHint("This was a read-only batch, so nothing was modified. Retry the failed refs or fix invalid references.").
		WithNextSteps("Inspect the failed items on stdout and follow each item's own next_steps")
	e.Retryable = retryable
	return e
}

// batchPartialFailure summarizes a batch with failed items. In a live batch
// the items marked ok were applied, so rerunning the whole batch would repeat
// them: the error is not retryable and its next step points at each item's
// own guidance. Its next steps are set explicitly, because the category's
// defaults ("Retry later") would invite exactly that rerun. A --dry-run batch
// sent nothing, so it says so instead.
func batchPartialFailure(cat cerrors.Category, failed, total int, dryRun bool) error {
	e := cerrors.Newf(cat, "BATCH_PARTIAL_FAILURE",
		"%d of %d operations failed; see per-item results on stdout", failed, total)
	if dryRun {
		return e.WithHint("This was a --dry-run preview, so nothing was sent; the items marked ok show the requests they would send.").
			WithNextSteps("Fix or drop the failed items, then preview again")
	}
	e = e.WithHint("Do not rerun the whole batch: the items marked ok were applied. " +
		"Retry only failed items whose error is retryable, and check any whose outcome is unknown with a read first.").
		WithNextSteps("Follow each failed item's own next_steps in the per-item results on stdout")
	e.Retryable = false
	return e
}
