package config_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/application/domain/secret"
	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/config"
)

// action.yml and this package are two statements of the same contract: the set
// of values a run is given, and what each is called. Nothing but care kept them
// in step, and care did not — the input names and the variable names drifted
// into four different conventions.
//
//	paypaysec-username   ->  PAYPAY_SEC_USERNAME     (different separator)
//	moneyforward-email   ->  MF_EMAIL                (different abbreviation)
//	account-id-hash      ->  MF_ASSET_ID             (different word entirely)
//	gmail-credentials    ->  GMAIL_CREDENTIALS_JSON  (extra suffix)
//
// Each was defensible where it was written. Together they meant a reader had to
// hold a translation table, and adding a value meant inventing two names and
// hoping.
//
// The rule now is one line: the variable is the input, upper-cased, with
// hyphens as underscores. These tests are what keeps it true.

var (
	inputHeadPattern = regexp.MustCompile(`(?m)^  ([a-z0-9-]+):$`)
	actionEnvPattern = regexp.MustCompile(`(?m)^\s+([A-Z0-9_]+): \$\{\{ inputs\.([a-z0-9-]+) \}\}$`)
)

// buildKnobs are inputs that are not credentials and never reach the
// environment. Listed rather than inferred: an input silently exempting itself
// is how a credential would go unchecked.
var buildKnobs = map[string]bool{"go-version-file": true}

// envNameFor is the rule, as a function.
func envNameFor(input string) string {
	return strings.ToUpper(strings.ReplaceAll(input, "-", "_"))
}

func TestActionEnvNamesFollowTheInputNames(t *testing.T) {
	body := readActionYML(t)

	pairs := actionEnvPattern.FindAllStringSubmatch(body, -1)
	if len(pairs) == 0 {
		t.Fatal("no `NAME: ${{ inputs.x }}` mappings found in action.yml; the parser or the file changed")
	}
	for _, m := range pairs {
		env, input := m[1], m[2]
		if want := envNameFor(input); env != want {
			t.Errorf("action.yml maps %s to %s; the rule says %s", input, env, want)
		}
	}
}

// TestEveryCredentialInputIsRead ties the action's declared inputs to what the
// program actually reads.
//
// An input nothing reads is a value a caller is asked for and never given a use
// for. A variable read but not declared is worse: it works for whoever set it
// by hand and fails for everyone using the action as documented.
func TestEveryCredentialInputIsRead(t *testing.T) {
	declared := map[string]bool{}
	for name := range credentialInputs(t) {
		declared[envNameFor(name)] = true
	}

	read := map[string]bool{config.GmailCredentials: true}
	for _, name := range secret.RequiredNames() {
		read[name] = true
	}

	for name := range declared {
		if !read[name] {
			t.Errorf("action.yml declares %s but nothing reads it", name)
		}
	}
	for name := range read {
		if !declared[name] {
			t.Errorf("%s is read but action.yml does not declare it; a caller using "+
				"the action has no way to supply it", name)
		}
	}
}

// TestEveryCredentialInputIsRequired guards the failure mode a missing
// `required: true` produces: the action runs, the variable is empty, and the
// run fails at whichever step first needs it rather than at its declaration.
func TestEveryCredentialInputIsRequired(t *testing.T) {
	for name, body := range credentialInputs(t) {
		if !strings.Contains(body, "required: true") {
			t.Errorf("input %s is not required, so a caller omitting it gets an empty "+
				"value instead of an error", name)
		}
	}
}

// credentialInputs returns each credential input in action.yml and the body of
// its declaration.
//
// Scoped to the inputs block on purpose. A pattern loose enough to run over the
// whole file also matches `steps:` under `runs:`, which is how the first
// version of this test reported that the action declares an input called STEPS.
func credentialInputs(t *testing.T) map[string]string {
	t.Helper()
	body := readActionYML(t)

	start := strings.Index(body, "\ninputs:\n")
	end := strings.Index(body, "\nruns:\n")
	if start < 0 || end < 0 || end < start {
		t.Fatal("could not find the inputs block in action.yml")
	}
	block := body[start:end]

	heads := inputHeadPattern.FindAllStringSubmatchIndex(block, -1)
	if len(heads) == 0 {
		t.Fatal("no inputs found in action.yml; the parser or the file changed")
	}

	out := map[string]string{}
	for i, h := range heads {
		name := block[h[2]:h[3]]
		if buildKnobs[name] {
			continue
		}
		stop := len(block)
		if i+1 < len(heads) {
			stop = heads[i+1][0]
		}
		out[name] = block[h[1]:stop]
	}
	return out
}

func readActionYML(t *testing.T) string {
	t.Helper()
	// From internal/config to the repository root.
	b, err := os.ReadFile("../../action.yml")
	if err != nil {
		t.Fatalf("read action.yml: %v", err)
	}
	return string(b)
}
