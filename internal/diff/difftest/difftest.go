// Package difftest holds shared diff fixtures for tests: real PR diffs and
// hand-written edge cases. Only test code should import it.
package difftest

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"strings"
)

//go:embed testdata
var files embed.FS

// Fixture is a named raw unified diff.
type Fixture struct {
	Name string
	Raw  string
}

// RawDiffs returns every raw-diff fixture.
func RawDiffs() []Fixture {
	return []Fixture{
		{Name: "edgecases", Raw: mustRead("testdata/edgecases.diff")},
		{Name: "ts64457", Raw: mustRead("testdata/ts64457.diff.gz")},
	}
}

// LongLineDiff returns a diff whose second hunk follows a line longer than
// bufio.Scanner's default 64KB token limit.
func LongLineDiff() Fixture {
	long := strings.Repeat("x", 70000)
	raw := "diff --git a/a.min.js b/a.min.js\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/a.min.js\n" +
		"+++ b/a.min.js\n" +
		"@@ -1,3 +1,3 @@\n" +
		" ctx\n" +
		"-" + long + "\n" +
		"+" + long + "y\n" +
		" tail\n" +
		"@@ -10,2 +10,2 @@\n" +
		"-old\n" +
		"+new\n" +
		" end\n" +
		"diff --git a/after.go b/after.go\n" +
		"index 3333333..4444444 100644\n" +
		"--- a/after.go\n" +
		"+++ b/after.go\n" +
		"@@ -1 +1 @@\n" +
		"-a\n" +
		"+b\n"
	return Fixture{Name: "longline", Raw: raw}
}

// FilesJSON returns a recorded "List pull request files" API response
// (a JSON array). name is "ts64457" or "k8s142410".
func FilesJSON(name string) []byte {
	return []byte(mustRead("testdata/" + name + "_files.json.gz"))
}

func mustRead(path string) string {
	b, err := files.ReadFile(path)
	if err != nil {
		panic(err)
	}
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			panic(err)
		}
		if b, err = io.ReadAll(zr); err != nil {
			panic(err)
		}
	}
	return string(b)
}
