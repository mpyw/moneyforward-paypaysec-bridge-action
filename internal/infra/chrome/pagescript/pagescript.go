// Package pagescript turns embedded JavaScript into expressions a page can
// evaluate.
//
// Its own package, and a type rather than loose functions, because two things
// here are easy to get wrong and both are invisible until a page misbehaves.
// Scripts have to be found — a name that does not exist is a build mistake, not
// a runtime condition — and arguments have to reach the page intact. Selectors
// are full of quotes and brackets, and splicing one into source text is how they
// arrive mangled.
package pagescript

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// Set is a collection of scripts, read once from an embedded filesystem.
//
// Each owner keeps its own: the browser layer's generic probes and a site's
// extraction routines have no reason to share a namespace, and a Set makes that
// separation the default rather than a convention.
type Set struct {
	dir     string
	scripts map[string]string
}

// Load reads every file under dir.
//
// It panics on failure. The files come from a go:embed directive, so a missing
// or unreadable one means the binary was built wrong — a condition worth
// surfacing at init rather than threading an error through every call site that
// can do nothing about it.
func Load(fsys fs.FS, dir string) *Set {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		panic("pagescript: read " + dir + ": " + err.Error())
	}

	set := &Set{dir: dir, scripts: make(map[string]string, len(entries))}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, rerr := fs.ReadFile(fsys, dir+"/"+e.Name())
		if rerr != nil {
			panic("pagescript: read " + dir + "/" + e.Name() + ": " + rerr.Error())
		}
		set.scripts[e.Name()] = string(body)
	}
	if len(set.scripts) == 0 {
		panic("pagescript: no scripts under " + dir)
	}
	return set
}

// Names lists the scripts in the set.
func (s *Set) Names() []string {
	out := make([]string, 0, len(s.scripts))
	for name := range s.scripts {
		out = append(out, name)
	}
	return out
}

// Source returns a script's text, panicking if the name is unknown.
func (s *Set) Source(name string) string {
	body, ok := s.scripts[name]
	if !ok {
		panic("pagescript: no script named " + name + " under " + s.dir)
	}
	return body
}

// Call renders a script applied to args, ready for chromedp.Evaluate.
//
// Every script is a function expression, so arguments cross into the page as
// JSON rather than by being pasted into the source. That keeps selector strings,
// quotes and backslashes intact, and keeps the .js files independent of whatever
// the Go side happens to call things.
func (s *Set) Call(name string, args ...any) (string, error) {
	return Apply(s.Source(name), args...)
}

// Apply renders an arbitrary function expression applied to args.
//
// Exported separately from [Set.Call] for the occasional one-line expression
// that does not warrant a file of its own.
func Apply(fn string, args ...any) (string, error) {
	encoded := make([]string, 0, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return "", fmt.Errorf("pagescript: encode argument %d: %w", i, err)
		}
		encoded = append(encoded, string(b))
	}
	return fmt.Sprintf("(%s)(%s)", strings.TrimSpace(fn), strings.Join(encoded, ", ")), nil
}
