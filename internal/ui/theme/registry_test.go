package theme

import (
	"reflect"
	"strings"
	"testing"
)

func TestGetResolvesDefaultAndAliases(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "pho-default", "pho", "default", "  Pho-Default "} {
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

	for _, name := range []string{"solarized-nope", "pho-nope"} {
		if th, ok := Get(name); ok || th != nil {
			t.Errorf("Get(%q) = %v, %v; want nil, false", name, th, ok)
		}
		if Exists(name) {
			t.Errorf("Exists(%q) = true", name)
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
			t.Error("expected a panic registering pho-default twice")
		}
	}()
	Register(phoDefault)
}

func TestRegisterRejectsAliases(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"pho", " Default"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic registering %q, an alias", name)
				}
			}()
			p := phoDefault
			p.Name = name
			Register(p)
		}()
	}
}

func TestRegisterNormalizesName(t *testing.T) {
	// Not parallel: it adds to the registry and removes the entry again.
	p := phoDefault
	p.Name = "  Test-Theme "
	Register(p)
	t.Cleanup(func() { delete(palettes, "test-theme") })

	th, ok := Get("test-theme")
	if !ok || th.Name != "test-theme" {
		t.Errorf("Get(test-theme) = %v, %v; want Name test-theme", th, ok)
	}
}

func TestNameListIncludesAliases(t *testing.T) {
	t.Parallel()

	got := NameList()
	for _, want := range []string{DefaultName, "pho", "default"} {
		if !strings.Contains(got, want) {
			t.Errorf("NameList() = %q, missing %q", got, want)
		}
	}
}
