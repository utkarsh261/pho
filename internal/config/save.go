package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/utkarsh261/pho/internal/fsutil"
)

// ErrManualEdit means the config defines [ui] in a form SaveTheme does not
// rewrite (an inline table, say), so the user has to set the theme by hand.
var ErrManualEdit = errors.New("config: [ui] is written in a form pho cannot edit")

var themeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// SaveTheme sets [ui] theme = name in the config at path (the XDG default
// when empty). It edits the one line involved so comments and layout
// survive, writes through symlinks to the real file, checks the result
// decodes to the same config apart from the theme, and replaces the file
// atomically. On any error the file is left untouched.
func SaveTheme(path, name string) error {
	if !themeNameRe.MatchString(name) {
		return fmt.Errorf("config: invalid theme name %q", name)
	}
	target, err := writeTarget(ResolvePaths(path).ConfigFile)
	if err != nil {
		return err
	}

	old, mode, inPlace := "", fs.FileMode(0o600), false
	switch data, err := os.ReadFile(target); {
	case err == nil:
		old = string(data)
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("config: stat %s: %w", target, err)
		}
		mode = info.Mode().Perm()
		if mode&0o222 == 0 {
			return fmt.Errorf("config: %s is read-only", target)
		}
		// Replacing a hard-linked file (say, into a dotfiles repo) would
		// cut the link, so those are rewritten in place instead.
		inPlace = linkCount(info) > 1
	case errors.Is(err, fs.ErrNotExist):
	default:
		return fmt.Errorf("config: read %s: %w", target, err)
	}

	updated, err := setTheme(old, name)
	if err != nil {
		return err
	}
	if err := verifyThemeEdit(old, updated, name); err != nil {
		return err
	}
	if inPlace {
		if err := os.WriteFile(target, []byte(updated), mode); err != nil {
			return fmt.Errorf("config: write %s: %w", target, err)
		}
		return nil
	}
	if err := fsutil.WriteAtomic(target, []byte(updated), mode); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

// writeTarget resolves the regular file SaveTheme should write, following a
// symlinked file or directory so the link itself is never replaced.
func writeTarget(path string) (string, error) {
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.Mode()&fs.ModeSymlink == 0:
		return path, nil
	case err == nil: // a symlink
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("config: resolve %s: %w", path, err)
		}
		// Dangling: create the file the link points at.
		dest, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("config: resolve %s: %w", path, err)
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(path), dest)
		}
		if info, err := os.Lstat(dest); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("config: resolve %s: chained dangling symlink", path)
		}
		return newFileIn(dest)
	case errors.Is(err, fs.ErrNotExist):
		return newFileIn(path)
	default:
		return "", fmt.Errorf("config: stat %s: %w", path, err)
	}
}

// newFileIn creates path's directory and returns path with that directory's
// symlinks resolved.
func newFileIn(path string) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("config: create %s: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("config: resolve %s: %w", dir, err)
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}

var (
	headerRe      = regexp.MustCompile(`^\s*\[\s*([^\[\]]+?)\s*\]\s*(#.*)?$`)
	arrayHeaderRe = regexp.MustCompile(`^\s*\[\[.*\]\]\s*(#.*)?$`)
	// Keys may be bare or quoted: theme, "theme" or 'theme'.
	tableThemeRe  = regexp.MustCompile(`^(\s*(?:theme|"theme"|'theme')\s*=\s*)("[^"\\]*"|'[^']*')(.*)$`)
	dottedRe      = regexp.MustCompile(`^\s*(?:ui|"ui"|'ui')\s*\.`)
	dottedThemeRe = regexp.MustCompile(`^(\s*(?:ui|"ui"|'ui')\s*\.\s*(?:theme|"theme"|'theme')\s*=\s*)("[^"\\]*"|'[^']*')(.*)$`)
	inlineUIRe    = regexp.MustCompile(`^\s*(?:ui|"ui"|'ui')\s*=`)
)

// setTheme returns content with the theme set, editing a single line.
func setTheme(content, name string) (string, error) {
	// A byte-order mark stays where it is, out of the way of the matching.
	if rest, ok := strings.CutPrefix(content, "\ufeff"); ok {
		out, err := setTheme(rest, name)
		return "\ufeff" + out, err
	}
	quoted := `"` + name + `"`
	if strings.TrimSpace(content) == "" {
		return "[ui]\ntheme = " + quoted + "\n", nil
	}

	nl := "\n"
	if strings.Contains(content, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(content, "\n")
	text := func(i int) string { return strings.TrimSuffix(lines[i], "\r") }
	cr := func(i int) string {
		if strings.HasSuffix(lines[i], "\r") {
			return "\r"
		}
		return ""
	}
	replaceValue := func(i int, re *regexp.Regexp) string {
		m := re.FindStringSubmatch(text(i))
		value := quoted
		if strings.HasPrefix(m[2], "'") {
			value = "'" + name + "'"
		}
		lines[i] = m[1] + value + m[3] + cr(i)
		return strings.Join(lines, "\n")
	}
	insertAfter := func(i int, line string) string {
		lines = append(lines[:i+1], append([]string{line + cr(i)}, lines[i+1:]...)...)
		return strings.Join(lines, "\n")
	}

	section := "" // current table; "" is the root
	uiHeader, lastRootUI := -1, -1
	for i := range lines {
		line := text(i)
		if arrayHeaderRe.MatchString(line) {
			section = "[[array]]" // never [ui] or the root
			continue
		}
		if m := headerRe.FindStringSubmatch(line); m != nil {
			section = tableName(m[1])
			if section == "ui" {
				uiHeader = i
			}
			continue
		}
		switch section {
		case "ui":
			if tableThemeRe.MatchString(line) {
				return replaceValue(i, tableThemeRe), nil
			}
		case "":
			if dottedThemeRe.MatchString(line) {
				return replaceValue(i, dottedThemeRe), nil
			}
			if dottedRe.MatchString(line) {
				lastRootUI = i
			}
			if inlineUIRe.MatchString(line) {
				return "", ErrManualEdit
			}
		}
	}

	switch {
	case uiHeader >= 0:
		return insertAfter(uiHeader, "theme = "+quoted), nil
	case lastRootUI >= 0:
		return insertAfter(lastRootUI, "ui.theme = "+quoted), nil
	}
	out := content
	if !strings.HasSuffix(out, "\n") {
		out += nl
	}
	return out + nl + "[ui]" + nl + "theme = " + quoted + nl, nil
}

// tableName normalises a [header]'s name: whitespace and quotes around each
// dotted part are dropped, so [ "ui" ] and ['ui'] both read as ui.
func tableName(header string) string {
	parts := strings.Split(header, ".")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(p), `"'`)
	}
	return strings.Join(parts, ".")
}

// verifyThemeEdit checks updated decodes to old's config with only
// ui.theme changed to name.
func verifyThemeEdit(old, updated, name string) error {
	var before, after map[string]any
	if _, err := toml.Decode(old, &before); err != nil {
		return fmt.Errorf("config: parse current config: %w", err)
	}
	if _, err := toml.Decode(updated, &after); err != nil {
		return ErrManualEdit
	}
	ui, _ := after["ui"].(map[string]any)
	if ui == nil || ui["theme"] != name {
		return ErrManualEdit
	}
	delete(ui, "theme")
	if beforeUI, ok := before["ui"].(map[string]any); ok {
		delete(beforeUI, "theme")
	} else if len(ui) == 0 {
		delete(after, "ui")
	}
	if !reflect.DeepEqual(before, after) {
		return ErrManualEdit
	}
	return nil
}
