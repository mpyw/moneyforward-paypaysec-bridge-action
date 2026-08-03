package pagescript_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mpyw/moneyforward-paypaysec-bridge-action/internal/infra/chrome/pagescript"
)

func testSet(t *testing.T) *pagescript.Set {
	t.Helper()
	return pagescript.Load(fstest.MapFS{
		"js/echo.js":  {Data: []byte("(x) => x")},
		"js/plain.js": {Data: []byte("() => 1")},
	}, "js")
}

func TestApply(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		args []any
		want string
	}{
		{"no arguments", "() => 1", nil, "(() => 1)()"},
		{"one map", "(s) => s", []any{map[string]string{"a": "b"}}, `((s) => s)({"a":"b"})`},
		{"two arguments", "(a, b) => a", []any{"x", 3}, `((a, b) => a)("x", 3)`},
		{
			// The reason arguments are JSON rather than spliced text: a selector
			// full of quotes and brackets has to survive into the page intact.
			name: "a selector with quotes and brackets",
			fn:   "(s) => s",
			args: []any{`input[name="mfid_user[email]"]`},
			want: `((s) => s)("input[name=\"mfid_user[email]\"]")`,
		},
		{"surrounding whitespace is trimmed", "\n  () => 1\n", nil, "(() => 1)()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pagescript.Apply(tt.fn, tt.args...)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Apply() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyRejectsUnencodableArguments(t *testing.T) {
	if _, err := pagescript.Apply("(x) => x", make(chan int)); err == nil {
		t.Fatal("Apply() accepted an argument that cannot be JSON encoded")
	}
}

func TestSetCall(t *testing.T) {
	got, err := testSet(t).Call("echo.js", "hi")
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if got != `((x) => x)("hi")` {
		t.Errorf("Call() = %q", got)
	}
}

func TestSetNames(t *testing.T) {
	names := testSet(t).Names()
	// The names, not the count: a Set that returned two of anything satisfied
	// the count, and the point of Names is which scripts were embedded.
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for _, want := range []string{"plain.js", "echo.js"} {
		if !got[want] {
			t.Errorf("Names() = %v, missing %q", names, want)
		}
	}
	if len(names) != 2 {
		t.Errorf("Names() = %v, want exactly the two embedded scripts", names)
	}
}

// TestSetSourcePanicsOnAnUnknownName treats a bad name as a build mistake: the
// files come from go:embed, so a name that is not there means the binary is
// wrong, not that something went wrong at runtime.
func TestSetSourcePanicsOnAnUnknownName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Source() with an unknown name did not panic")
		}
	}()
	testSet(t).Source("absent.js")
}

func TestLoadPanicsOnAnEmptyDirectory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Load() of a directory with no scripts did not panic")
		}
	}()
	pagescript.Load(fstest.MapFS{"js/.keep": {Data: []byte("")}}, "nope")
}

// TestEverySetMemberIsAFunctionExpression guards what Call assumes. A script
// that is not one fails inside a page, where the error is far less legible.
func TestEverySetMemberIsAFunctionExpression(t *testing.T) {
	set := testSet(t)
	for _, name := range set.Names() {
		if !strings.Contains(set.Source(name), "=>") {
			t.Errorf("%s does not look like a function expression", name)
		}
	}
}
