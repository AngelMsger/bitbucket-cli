package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/angelmsger/bitbucket-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/bitbucket-cli/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// reviewInputDoc is the JSON/YAML document accepted by `pr review --input`.
// Callers may pass either this object or a bare array of ReviewCommentInput.
type reviewInputDoc struct {
	Verdict              string                         `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	ExpectedSourceCommit string                         `json:"expected_source_commit,omitempty" yaml:"expected_source_commit,omitempty"`
	Comments             []apiclient.ReviewCommentInput `json:"comments,omitempty" yaml:"comments,omitempty"`
}

func newPRReviewCmd(s *appState) *cobra.Command {
	var (
		verdict              string
		expectedSourceCommit string
		inputPath            string
		dryRun               bool
	)
	cmd := &cobra.Command{
		Use:   "review <workspace>/<repo>/<id>",
		Short: "Validate and submit a composite review (comments + optional verdict)",
		Long: "Submit zero or more comments and an optional verdict in two phases.\n\n" +
			"Phase 1 validates everything before sending any write request: checks\n" +
			"`--expected-source-commit` against the remote PR head (returning\n" +
			"PR_HEAD_MOVED on mismatch), fetches the diff once, and resolves every\n" +
			"inline comment anchor. Any validation error aborts with zero remote writes.\n\n" +
			"Phase 2 posts each comment sequentially, rechecks the PR's source commit\n" +
			"immediately before voting (Bitbucket REST APIs have no single atomic\n" +
			"comments+vote transaction), and applies the verdict only if every comment\n" +
			"succeeded and the head has not moved. If a mid-flight write fails or the\n" +
			"head moves before voting, the partial result (posted_comments, failed_item,\n" +
			"vote_applied) is emitted on stdout before returning REVIEW_PARTIAL_FAILURE\n" +
			"so recovery can be idempotent.\n\n" +
			"Pass `--input <file>` or `--input -` (stdin) with either a JSON/YAML object\n" +
			"(`{verdict, expected_source_commit, comments: [...]}`) or a bare array of\n" +
			"comment objects (`{content, path?, line?, side?, reply_to?}`). Explicit CLI\n" +
			"flags override fields in `--input`.",
		Example: "  bitbucket-cli pr review myws/myrepo/7 --verdict approve --expected-source-commit abc1234 --dry-run\n" +
			"  bitbucket-cli pr review myws/myrepo/7 --verdict request-changes --expected-source-commit abc1234 --input review.json\n" +
			"  bitbucket-cli pr review myws/myrepo/7 --input - <<'EOF'\n" +
			"  [{\"path\":\"src/main.go\",\"line\":42,\"content\":\"Nil dereference here.\"}]\n" +
			"  EOF",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, id, err := resolvePRRef(args[0], apiclient.RepoRef{})
			if err != nil {
				return err
			}
			var doc reviewInputDoc
			if inputPath != "" {
				raw, err := readReviewInput(inputPath, cmd.InOrStdin())
				if err != nil {
					return err
				}
				doc, err = parseReviewInput(raw)
				if err != nil {
					return err
				}
			}
			finalVerdict := doc.Verdict
			if cmd.Flags().Changed("verdict") || verdict != "" {
				finalVerdict = verdict
			}
			if finalVerdict == "" && len(doc.Comments) > 0 {
				finalVerdict = string(apiclient.ReviewVerdictCommentOnly)
			}
			if finalVerdict == "" {
				return cerrors.New(cerrors.CategoryUsage, "REVIEW_EMPTY",
					"pass --verdict and/or --input with comments to submit a review").
					WithNextSteps("Pass --verdict approve|request-changes|comment-only or --input <file|->")
			}
			normVerdict, err := apiclient.NormalizeReviewVerdict(finalVerdict)
			if err != nil {
				return err
			}
			finalCommit := doc.ExpectedSourceCommit
			if cmd.Flags().Changed("expected-source-commit") || expectedSourceCommit != "" {
				finalCommit = expectedSourceCommit
			}

			req := apiclient.SubmitPRReviewReq{
				Repo:                 ref,
				PRID:                 id,
				Verdict:              normVerdict,
				ExpectedSourceCommit: finalCommit,
				Comments:             doc.Comments,
			}

			ctx, cancel := cmdContext(s)
			defer cancel()
			client, err := s.newClient(ctx)
			if err != nil {
				return err
			}
			if dryRun {
				plan, err := client.PlanPRReview(ctx, req)
				if err != nil {
					return err
				}
				return s.emit(plan)
			}
			res, err := client.SubmitPRReview(ctx, req)
			if err != nil {
				if res != nil {
					// Emit the partial outcome on stdout so callers see every
					// posted comment ID and whether the vote ran before handling
					// the structured error on stderr.
					if emitErr := s.emit(res); emitErr != nil {
						return emitErr
					}
				}
				return err
			}
			return s.emit(res)
		},
	}
	f := cmd.Flags()
	f.StringVar(&verdict, "verdict", "", "approve | request-changes | comment-only")
	f.StringVar(&expectedSourceCommit, "expected-source-commit", "",
		"abort with PR_HEAD_MOVED if the remote PR source commit does not match this SHA")
	f.StringVar(&inputPath, "input", "",
		"JSON or YAML file describing comments (and optional verdict/expected_source_commit); use '-' for stdin")
	f.BoolVar(&dryRun, "dry-run", false, "validate anchors and preview all HTTP requests without sending writes")
	enumComplete(cmd, "verdict", "approve", "request-changes", "comment-only")
	return cmd
}

func readReviewInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_STDIN",
				"could not read --input from stdin")
		}
		return b, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_FILE",
			"could not read --input file")
	}
	return b, nil
}

// parseReviewInput parses either a JSON/YAML object ({verdict,
// expected_source_commit, comments}) or a bare JSON/YAML array of comment
// objects (including YAML flow style such as `[{content: hello}]`). Valid JSON
// is decoded strictly with encoding/json (preserving RFC 8259 UTF-16 surrogate
// escapes and rejecting fractional numbers for integer fields); non-JSON inputs
// fall back to YAML 1.2 with matching strict integer and single-document checks.
func parseReviewInput(raw []byte) (reviewInputDoc, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return reviewInputDoc{}, cerrors.New(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"--input content is empty")
	}

	if json.Valid(trimmed) {
		switch trimmed[0] {
		case '[':
			var comments []apiclient.ReviewCommentInput
			dec := json.NewDecoder(bytes.NewReader(trimmed))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&comments); err != nil {
				return reviewInputDoc{}, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
					"could not parse --input JSON comment array")
			}
			if err := ensureSingleJSONDoc(dec); err != nil {
				return reviewInputDoc{}, err
			}
			return reviewInputDoc{Comments: comments}, nil
		case '{':
			var doc reviewInputDoc
			dec := json.NewDecoder(bytes.NewReader(trimmed))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&doc); err != nil {
				return reviewInputDoc{}, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
					"could not parse --input JSON object")
			}
			if err := ensureSingleJSONDoc(dec); err != nil {
				return reviewInputDoc{}, err
			}
			return doc, nil
		}
	}

	// Non-JSON input (including YAML block and flow collections such as
	// `[{content: hello}]`): decode AST first to verify single-document EOF,
	// enforce integer-only scalar types on line/reply_to, and route by kind.
	nodeDec := yaml.NewDecoder(bytes.NewReader(trimmed))
	var node yaml.Node
	if err := nodeDec.Decode(&node); err != nil {
		return reviewInputDoc{}, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"could not parse --input as JSON or YAML")
	}
	if err := ensureSingleYAMLDoc(nodeDec); err != nil {
		return reviewInputDoc{}, err
	}
	if len(node.Content) == 0 {
		return reviewInputDoc{}, cerrors.New(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"--input content is empty")
	}

	root := node.Content[0]
	if err := rejectNonIntegerCommentFields(root); err != nil {
		return reviewInputDoc{}, err
	}
	switch root.Kind {
	case yaml.SequenceNode:
		var comments []apiclient.ReviewCommentInput
		dec := yaml.NewDecoder(bytes.NewReader(trimmed))
		dec.KnownFields(true)
		if err := dec.Decode(&comments); err != nil {
			return reviewInputDoc{}, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
				"could not parse --input comment list")
		}
		if err := ensureSingleYAMLDoc(dec); err != nil {
			return reviewInputDoc{}, err
		}
		return reviewInputDoc{Comments: comments}, nil
	case yaml.MappingNode:
		var doc reviewInputDoc
		dec := yaml.NewDecoder(bytes.NewReader(trimmed))
		dec.KnownFields(true)
		if err := dec.Decode(&doc); err != nil {
			return reviewInputDoc{}, cerrors.Wrap(err, cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
				"could not parse --input review object")
		}
		if err := ensureSingleYAMLDoc(dec); err != nil {
			return reviewInputDoc{}, err
		}
		return doc, nil
	default:
		return reviewInputDoc{}, cerrors.Newf(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"--input must be a JSON/YAML object or array of comments (got %s)", strings.ToLower(string(trimmed[:min(len(trimmed), 24)])))
	}
}

// rejectNonIntegerCommentFields prevents gopkg.in/yaml.v3 from silently
// truncating float scalars (such as `reply_to: 42.9` -> 42) into Go int fields.
func rejectNonIntegerCommentFields(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	switch n.Kind {
	case yaml.SequenceNode:
		for _, child := range n.Content {
			if err := rejectNonIntegerCommentFields(child); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Value == "reply_to" || k.Value == "line" {
				if v.Kind == yaml.ScalarNode && v.Tag == "!!float" {
					return cerrors.Newf(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
						"field %q must be an integer, got %q", k.Value, v.Value)
				}
			}
			if err := rejectNonIntegerCommentFields(v); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureSingleJSONDoc(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return cerrors.New(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"unexpected trailing content after JSON document in --input")
	}
	return nil
}

func ensureSingleYAMLDoc(dec *yaml.Decoder) error {
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return cerrors.New(cerrors.CategoryUsage, "BAD_REVIEW_INPUT",
			"unexpected extra document or trailing content in --input")
	}
	return nil
}
