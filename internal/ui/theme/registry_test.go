package theme

import (
	"reflect"
	"testing"
)

func TestGetResolvesDefaultAndAliases(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"terminal-workshop", "pho", "default", "  Terminal-Workshop "} {
		th, ok := Get(name)
		if !ok {
			t.Errorf("Get(%q) not found", name)
			continue
		}
		if !reflect.DeepEqual(th, Default()) {
			t.Errorf("Get(%q) differs from Default()", name)
		}
	}
}

func TestGetUnknown(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "solarized-nope"} {
		if th, ok := Get(name); ok || th != nil {
			t.Errorf("Get(%q) = %v, %v; want nil, false", name, th, ok)
		}
	}
}

func TestNamesStartsWithDefault(t *testing.T) {
	t.Parallel()

	names := Names()
	if len(names) == 0 || names[0] != DefaultName {
		t.Fatalf("Names() = %v, want %q first", names, DefaultName)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("Names() lists %q twice", n)
		}
		seen[n] = true
		if _, ok := Get(n); !ok {
			t.Errorf("Names() lists %q but Get cannot find it", n)
		}
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("expected a panic registering terminal-workshop twice")
		}
	}()
	Register(terminalWorkshop)
}
