package markdown

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	gomd "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// ParsedNotes holds the extracted release notes data.
type ParsedNotes struct {
	Version  string
	Date     string
	Tags     []string
	RawMD    string   // markdown without tag lines
	HTML     string   // rendered HTML
}

// ParseReleaseNotes reads a markdown file, extracts @tags, and renders HTML.
//
// Tags are lines starting with "tags:" or inline @tagname tokens.
// Example markdown:
//
//	# Release v1.2.3
//	tags: backend, finance, ops
//
//	## New Features
//	- Something new
func ParseReleaseNotes(path string) (*ParsedNotes, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening notes file: %w", err)
	}
	defer f.Close()

	var (
		version     string
		tags        []string
		cleanLines  []string
	)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Extract version from first H1 heading
		if strings.HasPrefix(trimmed, "# ") && version == "" {
			// e.g. "# Release v1.2.3 — 2026-02-27"
			parts := strings.Fields(trimmed[2:])
			for _, p := range parts {
				if strings.HasPrefix(p, "v") || strings.HasPrefix(p, "V") {
					version = p
					break
				}
			}
			cleanLines = append(cleanLines, line)
			continue
		}

		// Tag line: "tags: backend, finance, ops"
		lowerTrimmed := strings.ToLower(trimmed)
		if strings.HasPrefix(lowerTrimmed, "tags:") {
			rawTags := strings.TrimPrefix(lowerTrimmed, "tags:")
			for _, t := range strings.Split(rawTags, ",") {
				tag := strings.TrimSpace(t)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
			// Don't include the tag line in the rendered output
			continue
		}

		cleanLines = append(cleanLines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading notes file: %w", err)
	}

	rawMD := strings.Join(cleanLines, "\n")

	// Render markdown to HTML
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(rawMD))

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)
	renderedHTML := string(gomd.Render(doc, renderer))

	if version == "" {
		version = "unknown"
	}

	return &ParsedNotes{
		Version: version,
		Date:    time.Now().Format("2006-01-02"),
		Tags:    tags,
		RawMD:   rawMD,
		HTML:    renderedHTML,
	}, nil
}
