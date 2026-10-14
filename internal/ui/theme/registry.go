package theme

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultName is the theme used when none is configured.
const DefaultName = "terminal-workshop"

var (
	palettes = map[string]Palette{}
	// aliases map alternative names onto registered palettes.
	aliases = map[string]string{
		"pho":     DefaultName,
		"default": DefaultName,
	}
)

func init() {
	Register(terminalWorkshop)
}

// Register adds p to the themes Get can return, under its name lower-cased
// and trimmed (which becomes p.Name). It panics on an empty or duplicate
// name, or one taken by an alias, since palettes are registered at init
// time.
func Register(p Palette) {
	name := normalizeName(p.Name)
	if name == "" {
		panic("theme: Register with empty palette name")
	}
	if _, alias := aliases[name]; alias {
		panic(fmt.Sprintf("theme: palette name %q is an alias", name))
	}
	p.Name = name
	if _, dup := palettes[name]; dup {
		panic(fmt.Sprintf("theme: palette %q registered twice", name))
	}
	palettes[name] = p
}

// Get builds the named theme. Names are case-insensitive, an empty name and
// the aliases "pho" and "default" mean Terminal Workshop, and ok is false
// for unknown names.
func Get(name string) (th *Theme, ok bool) {
	p, ok := lookup(name)
	if !ok {
		return nil, false
	}
	return Build(p), true
}

// Exists reports whether Get would find name, without building the theme.
func Exists(name string) bool {
	_, ok := lookup(name)
	return ok
}

func lookup(name string) (Palette, bool) {
	name = normalizeName(name)
	if name == "" {
		name = DefaultName
	}
	if target, isAlias := aliases[name]; isAlias {
		name = target
	}
	p, ok := palettes[name]
	return p, ok
}

// NameList describes the valid names for help and error text, e.g.
// "terminal-workshop (also: default, pho), nord".
func NameList() string {
	var also []string
	for alias := range aliases {
		also = append(also, alias)
	}
	sort.Strings(also)
	names := Names()
	names[0] += " (also: " + strings.Join(also, ", ") + ")"
	return strings.Join(names, ", ")
}

// Names lists the registered themes, the default first and the rest sorted.
func Names() []string {
	names := make([]string, 0, len(palettes))
	for name := range palettes {
		if name != DefaultName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return append([]string{DefaultName}, names...)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
