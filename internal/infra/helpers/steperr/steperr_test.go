package steperr

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapAndOf(t *testing.T) {
	cause := errors.New("boom")
	err := Wrap("await-home", cause)

	if got := Of(err); got != "await-home" {
		t.Errorf("Of() = %q, want %q", got, "await-home")
	}
	if !errors.Is(err, cause) {
		t.Error("the cause is not reachable through errors.Is")
	}
	if msg := err.Error(); !strings.Contains(msg, "await-home") || !strings.Contains(msg, "boom") {
		t.Errorf("Error() = %q, want both the step and the cause", msg)
	}
}

// TestWrapPassesNilThrough lets Wrap take a call's result directly, rather than
// every caller guarding it.
func TestWrapPassesNilThrough(t *testing.T) {
	if err := Wrap("navigate", nil); err != nil {
		t.Errorf("Wrap(nil) = %v, want nil", err)
	}
}

func TestOfOnAPlainError(t *testing.T) {
	if got := Of(errors.New("plain")); got != "" {
		t.Errorf("Of() = %q, want empty for an error carrying no step", got)
	}
}

// TestOfFindsAStepThroughWrapping covers the usual shape: a step error wrapped
// again with context by the caller that raised it.
func TestOfFindsAStepThroughWrapping(t *testing.T) {
	inner := Wrap("submit-otp", errors.New("timeout"))
	outer := errors.Join(errors.New("moneyforward: login"), inner)
	if got := Of(outer); got != "submit-otp" {
		t.Errorf("Of() = %q, want the step to survive further wrapping", got)
	}
}
