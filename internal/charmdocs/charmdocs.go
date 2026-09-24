package charmdocs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/glamour/v2"
)

type Document struct {
	Slug  string
	Title string
	Path  string
	Body  string
}

type Catalog struct {
	root  string
	docs  map[string]Document
	slugs []string
}

func Load(start string) (Catalog, error) {
	docsDir, root, err := findDocsDir(start)
	if err != nil {
		return Catalog{}, err
	}

	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return Catalog{}, fmt.Errorf("read charm docs dir: %w", err)
	}

	catalog := Catalog{
		root:  root,
		docs:  make(map[string]Document, len(entries)),
		slugs: make([]string, 0, len(entries)),
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		slug := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		path := filepath.Join(docsDir, entry.Name())
		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			return Catalog{}, fmt.Errorf("read %s: %w", path, err)
		}

		body := string(bodyBytes)
		catalog.docs[slug] = Document{
			Slug:  slug,
			Title: titleFromBody(slug, body),
			Path:  path,
			Body:  body,
		}
		catalog.slugs = append(catalog.slugs, slug)
	}

	sort.Strings(catalog.slugs)
	if len(catalog.slugs) == 0 {
		return Catalog{}, fmt.Errorf("no markdown docs found in %s", docsDir)
	}

	return catalog, nil
}

func (c Catalog) Root() string {
	return c.root
}

func (c Catalog) DocsDir() string {
	if c.root == "" {
		return ""
	}
	return filepath.Join(c.root, "docs", "charm")
}

func (c Catalog) Slugs() []string {
	return append([]string(nil), c.slugs...)
}

func (c Catalog) Title(slug string) string {
	doc, ok := c.docs[slug]
	if !ok {
		return ""
	}
	return doc.Title
}

func (c Catalog) Document(slug string) (Document, bool) {
	doc, ok := c.docs[slug]
	return doc, ok
}

func (c Catalog) Preview(slug string, width, lines int) (string, error) {
	rendered, err := c.Render(slug, width)
	if err != nil {
		return "", err
	}
	if lines <= 0 {
		return "", nil
	}

	all := strings.Split(rendered, "\n")
	trimmed := make([]string, 0, min(lines, len(all)))
	for _, line := range all {
		if len(trimmed) == lines {
			break
		}
		if strings.TrimSpace(line) == "" && len(trimmed) == 0 {
			continue
		}
		trimmed = append(trimmed, strings.TrimRight(line, " \t"))
	}

	return strings.Join(trimmed, "\n"), nil
}

func (c Catalog) Render(slug string, width int) (string, error) {
	doc, ok := c.docs[slug]
	if !ok {
		return "", fmt.Errorf("unknown charm doc %q", slug)
	}
	if width < 24 {
		width = 24
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", fmt.Errorf("create glamour renderer: %w", err)
	}

	rendered, err := renderer.Render(doc.Body)
	if err != nil {
		return "", fmt.Errorf("render %s: %w", slug, err)
	}

	return strings.TrimSpace(rendered), nil
}

func findDocsDir(start string) (string, string, error) {
	if start == "" {
		return "", "", fmt.Errorf("start path is empty")
	}

	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", "", fmt.Errorf("resolve start path: %w", err)
	}

	info, err := os.Stat(absStart)
	if err != nil {
		return "", "", fmt.Errorf("stat %s: %w", absStart, err)
	}

	dir := absStart
	if !info.IsDir() {
		dir = filepath.Dir(absStart)
	}

	for {
		candidate := filepath.Join(dir, "docs", "charm")
		candidateInfo, err := os.Stat(candidate)
		if err == nil && candidateInfo.IsDir() {
			return candidate, dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("stat %s: %w", candidate, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", fmt.Errorf("could not find docs/charm from %s", absStart)
}

func titleFromBody(slug, body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return strings.ToUpper(slug)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
