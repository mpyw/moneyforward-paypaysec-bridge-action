package moneyforward

import (
	"strings"
	"testing"
)

func TestClientValidate(t *testing.T) {
	if err := (&Client{Email: "a@example.com", Password: "x"}).Validate(); err != nil {
		t.Errorf("Validate() on a complete client = %v", err)
	}
	err := (&Client{}).Validate()
	if err == nil {
		t.Fatal("Validate() with no credentials succeeded")
	}
	// Both names at once, so a misconfigured run does not fail one at a time.
	for _, want := range []string{"Email", "Password"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error %v does not name %s", err, want)
		}
	}
}
