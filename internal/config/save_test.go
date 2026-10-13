package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetThemeSyntax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
		wantErr        error
	}{
		{"empty file", "", "[ui]\ntheme = \"nord\"\n", nil},
		{"whitespace only", "  \n", "[ui]\ntheme = \"nord\"\n", nil},
		{"no ui section",
			"# my config\n[logging]\nlevel = \"debug\"\n",
			"# my config\n[logging]\nlevel = \"debug\"\n\n[ui]\ntheme = \"nord\"\n", nil},
		{"no trailing newline",
			"[logging]\nlevel = \"debug\"",
			"[logging]\nlevel = \"debug\"\n\n[ui]\ntheme = \"nord\"\n", nil},
		{"replace keeps comment and quotes",
			"[ui] # looks\n  theme = 'dracula'   # set by me\n[diff]\nmax_lines = 1\n",
			"[ui] # looks\n  theme = 'nord'   # set by me\n[diff]\nmax_lines = 1\n", nil},
		{"insert after header",
			"[ui] # looks\nbackground = \"terminal\"\n",
			"[ui] # looks\ntheme = \"nord\"\nbackground = \"terminal\"\n", nil},
		{"ui followed by subtable",
			"[ui]\n[ui.colors]\ntheme = \"not this one\"\n",
			"[ui]\ntheme = \"nord\"\n[ui.colors]\ntheme = \"not this one\"\n", nil},
		{"only a subtable",
			"[ui.colors]\nprimary = \"#fff\"\n",
			"[ui.colors]\nprimary = \"#fff\"\n\n[ui]\ntheme = \"nord\"\n", nil},
		{"dotted theme",
			"ui.theme = \"dracula\"\n[logging]\nlevel = \"info\"\n",
			"ui.theme = \"nord\"\n[logging]\nlevel = \"info\"\n", nil},
		{"dotted without theme",
			"ui.background = \"terminal\"\n[logging]\nlevel = \"info\"\n",
			"ui.background = \"terminal\"\nui.theme = \"nord\"\n[logging]\nlevel = \"info\"\n", nil},
		{"crlf",
			"[ui]\r\ntheme = \"dracula\"\r\n",
			"[ui]\r\ntheme = \"nord\"\r\n", nil},
		{"crlf insert",
			"[ui]\r\nbackground = \"theme\"\r\n",
			"[ui]\r\ntheme = \"nord\"\r\nbackground = \"theme\"\r\n", nil},
		{"inline table", "ui = { background = \"terminal\" }\n", "", ErrManualEdit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := setTheme(tc.in, "nord")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
			if err := verifyThemeEdit(tc.in, got, "nord"); err != nil {
				t.Errorf("verify rejected the edit: %v", err)
			}
		})
	}
}

func TestVerifyThemeEditRejectsCorruption(t *testing.T) {
	t.Parallel()

	old := "[logging]\nlevel = \"debug\"\n"
	for name, updated := range map[string]string{
		"other key changed": "[logging]\nlevel = \"info\"\n[ui]\ntheme = \"nord\"\n",
		"invalid toml":      "[logging]\nlevel = \"debug\"\n[ui]\ntheme = \"nord\n",
		"theme missing":     old,
	} {
		if err := verifyThemeEdit(old, updated, "nord"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func readTheme(t *testing.T, path string) string {
	t.Helper()
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return cfg.UI.Theme
}

func TestSaveThemePaths(t *testing.T) {
	t.Parallel()

	t.Run("regular file keeps mode", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte("# keep me\n[ui]\ntheme = \"dracula\"\n"), 0o640)
		if err := SaveTheme(path, "nord"); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "# keep me\n[ui]\ntheme = \"nord\"\n" {
			t.Errorf("content = %q", data)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
			t.Errorf("mode = %v, want 0640", info.Mode().Perm())
		}
	})

	t.Run("missing file in missing dir", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "a", "b", "config.toml")
		if err := SaveTheme(path, "nord"); err != nil {
			t.Fatal(err)
		}
		if got := readTheme(t, path); got != "nord" {
			t.Errorf("theme = %q", got)
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	})

	t.Run("missing file under symlinked dir", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		real := filepath.Join(root, "dotfiles")
		os.Mkdir(real, 0o755)
		link := filepath.Join(root, "pho")
		os.Symlink(real, link)
		if err := SaveTheme(filepath.Join(link, "config.toml"), "nord"); err != nil {
			t.Fatal(err)
		}
		if got := readTheme(t, filepath.Join(real, "config.toml")); got != "nord" {
			t.Errorf("real file theme = %q", got)
		}
		if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
			t.Error("directory link was replaced")
		}
	})

	t.Run("existing file symlink", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		real := filepath.Join(root, "real.toml")
		os.WriteFile(real, []byte("[ui]\ntheme = \"dracula\"\n"), 0o600)
		link := filepath.Join(root, "config.toml")
		os.Symlink("real.toml", link)
		if err := SaveTheme(link, "nord"); err != nil {
			t.Fatal(err)
		}
		if got := readTheme(t, real); got != "nord" {
			t.Errorf("target theme = %q", got)
		}
		if dest, err := os.Readlink(link); err != nil || dest != "real.toml" {
			t.Errorf("link changed: %q, %v", dest, err)
		}
	})

	for _, relative := range []bool{true, false} {
		t.Run("dangling file symlink", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			real := filepath.Join(root, "dotfiles", "pho.toml") // its dir is missing too
			dest := real
			if relative {
				dest = filepath.Join("dotfiles", "pho.toml")
			}
			link := filepath.Join(root, "config.toml")
			os.Symlink(dest, link)
			if err := SaveTheme(link, "nord"); err != nil {
				t.Fatal(err)
			}
			if got := readTheme(t, link); got != "nord" {
				t.Errorf("theme through link = %q", got)
			}
			if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
				t.Error("link was replaced")
			}
		})
	}

	t.Run("symlink loop", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
		os.Symlink(b, a)
		os.Symlink(a, b)
		if err := SaveTheme(a, "nord"); err == nil {
			t.Error("expected an error for a symlink loop")
		}
	})

	t.Run("read-only dir leaves file untouched", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" || os.Getuid() == 0 {
			t.Skip("permissions not enforced")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		orig := "[ui]\ntheme = \"dracula\"\n"
		os.WriteFile(path, []byte(orig), 0o600)
		os.Chmod(dir, 0o500)
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
		if err := SaveTheme(path, "nord"); err == nil {
			t.Error("expected an error writing into a read-only directory")
		}
		if data, _ := os.ReadFile(path); string(data) != orig {
			t.Errorf("file changed: %q", data)
		}
	})

	t.Run("inline table", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "config.toml")
		orig := "ui = { background = \"terminal\" }\n"
		os.WriteFile(path, []byte(orig), 0o600)
		if err := SaveTheme(path, "nord"); !errors.Is(err, ErrManualEdit) {
			t.Errorf("err = %v, want ErrManualEdit", err)
		}
		if data, _ := os.ReadFile(path); string(data) != orig {
			t.Errorf("file changed: %q", data)
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		t.Parallel()
		if err := SaveTheme(filepath.Join(t.TempDir(), "c.toml"), `x"\n[evil]`); err == nil {
			t.Error("expected an error for an invalid name")
		}
	})
}
