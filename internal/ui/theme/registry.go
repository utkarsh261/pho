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

// Register adds p to the themes Get can return. It panics on an empty or
// duplicate name, since palettes are registered at init time.
func Register(p Palette) {
	name := normalizeName(p.Name)
	if name == "" {
		panic("theme: Register with empty palette name")
	}
	if _, dup := palettes[name]; dup {
		panic(fmt.Sprintf("theme: palette %q registered twice", name))
	}
	palettes[name] = p
}

// Get builds the named theme. Names are case-insensitive, and the aliases
// "pho" and "default" mean Terminal Workshop. ok is false for unknown names.
func Get(name string) (th *Theme, ok bool) {
	name = normalizeName(name)
	if target, isAlias := aliases[name]; isAlias {
		name = target
	}
	p, ok := palettes[name]
	if !ok {
		return nil, false
	}
	return Build(p), true
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
