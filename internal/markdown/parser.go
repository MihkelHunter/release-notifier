package markdown

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MihkelHunter/release-notifier/internal/trello"
	gomd "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/joho/godotenv"
)

// ParsedNotes holds the extracted release notes data.
type ParsedNotes struct {
	Version string
	Date    string
	Tags    []string
	RawMD   string // markdown without tag lines
	HTML    string // rendered HTML
}

type TrelloCard struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
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
	// Resolve absolute path so we can find images relative to the .md file
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolving notes path: %w", err)
	}
	notesDir := filepath.Dir(absPath)

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("opening notes file: %w", err)
	}
	defer f.Close()

	var (
		version    string
		tags       []string
		cleanLines []string
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

	err = godotenv.Load()
	if err != nil {
		fmt.Errorf("Error loading .env file: %w", err)
	}

	apiKey := os.Getenv("API_KEY")
	token := os.Getenv("API_TOKEN")
	listId := os.Getenv("API_LISTID")

	client := trello.NewClient(apiKey, token)
	trelloData, err := client.GetListCards(listId)
	if err != nil {
		fmt.Errorf("Failed getting trello data: %w", err)
	}

	var cards []TrelloCard
	err = json.Unmarshal(trelloData, &cards)
	if err != nil {
		fmt.Errorf("Failed unmarhsaling trello data: %w", err)
	}

	// Split rawMD into lines
	lines := strings.Split(rawMD, "\n")

	var outputLines []string
	insideNewFeatures := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of New Features block
		if strings.HasPrefix(trimmed, "## Muudatused") {
			insideNewFeatures = true
			outputLines = append(outputLines, line)
			continue
		}

		// Detect next H2 or H1 heading, which ends the New Features block
		if insideNewFeatures && strings.HasPrefix(trimmed, "## ") {
			// Append Trello cards before leaving the section
			for _, card := range cards {
				outputLines = append(outputLines, fmt.Sprintf("- %s", card.Name))
			}
			outputLines = append(outputLines, "\n")
			insideNewFeatures = false
		}

		outputLines = append(outputLines, line)

		// If we're at the last line and still inside New Features, append cards
		if i == len(lines)-1 && insideNewFeatures {
			for _, card := range cards {
				outputLines = append(outputLines, fmt.Sprintf("- %s", card.Name))
			}
			insideNewFeatures = false
		}
	}

	// Join lines back into rawMD
	rawMD = strings.Join(outputLines, "\n")

	// Render markdown to HTML
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(rawMD))

	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)
	renderedHTML := string(gomd.Render(doc, renderer))

	// Embed any local images as Base64 data URIs so they display correctly in email
	renderedHTML, err = embedLocalImages(renderedHTML, notesDir)
	if err != nil {
		return nil, fmt.Errorf("embedding images: %w", err)
	}

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
