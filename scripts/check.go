// Repository checks deliberately never read protected secret files.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var secret = regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|sk-[A-Za-z0-9]{20,}`)

func protected(name string) bool {
	b := strings.ToLower(filepath.Base(name))
	if b == ".env.example" || b == ".env.sample" || b == ".env.template" {
		return false
	}
	return b == ".env" || strings.HasPrefix(b, ".env.") || strings.HasSuffix(b, ".pem") || b == "id_rsa" || b == "id_ed25519"
}

func inspect(name string, body []byte) error {
	if !utf8.Valid(body) {
		return errors.New("invalid UTF-8")
	}
	for _, r := range string(body) {
		if unicode.Is(unicode.Cf, r) {
			return errors.New("hidden Unicode character")
		}
	}
	if secret.Match(body) {
		return errors.New("possible secret (value redacted)")
	}
	if strings.HasSuffix(name, ".go") {
		formatted, err := format.Source(body)
		if err != nil {
			return errors.New("invalid Go source")
		}
		if !bytes.Equal(formatted, bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))) {
			return errors.New("Go source needs gofmt")
		}
	}
	if name == "go.mod" {
		for _, forbidden := range []string{"twilio", "sendgrid", "mailgun", "aws-sdk", "vonage"} {
			if strings.Contains(strings.ToLower(string(body)), forbidden) {
				return errors.New("real messaging dependency prohibited")
			}
		}
	}
	return nil
}

func check(root string) error {
	// Check names first, never the contents of tracked/staged secret files.
	cmd := exec.Command("git", "ls-files", "--cached", "-z")
	cmd.Dir = root
	names, err := cmd.Output()
	if err != nil {
		return errors.New("cannot inspect Git index")
	}
	for _, name := range strings.Split(string(names), "\x00") {
		if name != "" && (protected(name) || strings.HasPrefix(filepath.ToSlash(name), "ai-sessions/raw/")) {
			return errors.New("protected path in Git index (contents not read)")
		}
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("cannot enumerate repository")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return errors.New("cannot resolve repository path")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if protected(path) || entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "raw" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if protected(path) {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".md" && ext != ".sql" && ext != ".yml" && ext != ".json" && ext != ".sh" && ext != ".ts" && ext != ".vue" && ext != ".css" && ext != ".html" && filepath.Base(path) != "go.mod" && filepath.Base(path) != "Dockerfile" && filepath.Base(path) != "Makefile" && filepath.Base(path) != ".env.example" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read %s", rel)
		}
		if err := inspect(filepath.ToSlash(rel), body); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		return nil
	})
}

func main() {
	if err := check("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("repository safety and formatting checks passed")
}
