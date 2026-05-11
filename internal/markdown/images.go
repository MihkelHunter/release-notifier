package markdown

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// imgTagRegex matches <img src="..."> tags produced by the markdown renderer.
// It captures the src value so we can check if it's a local path.
var imgTagRegex = regexp.MustCompile(`<img\s+([^>]*?)src="([^"]+)"([^>]*?)>`)

// embedLocalImages scans the rendered HTML for <img src="..."> tags and
// replaces any local file paths with Base64 data URIs.
// Images referenced by http:// or https:// are left untouched.
// notesDir is the directory of the .md file — relative paths are resolved from there.
func embedLocalImages(renderedHTML, notesDir string) (string, error) {
	var embedErr error

	result := imgTagRegex.ReplaceAllStringFunc(renderedHTML, func(match string) string {
		if embedErr != nil {
			return match
		}

		parts := imgTagRegex.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}

		src := parts[2]

		// Leave remote URLs as-is
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			return match
		}

		// Resolve relative path from the notes file directory
		imgPath := src
		if !filepath.IsAbs(imgPath) {
			imgPath = filepath.Join(notesDir, src)
		}

		dataURI, err := toBase64DataURI(imgPath)
		if err != nil {
			embedErr = fmt.Errorf("embedding image %q: %w", src, err)
			return match
		}

		// Replace just the src value, keeping all other attributes intact
		return strings.Replace(match, `src="`+src+`"`, `src="`+dataURI+`"`, 1)
	})

	if embedErr != nil {
		return "", embedErr
	}

	return result, nil
}

// toBase64DataURI reads an image file and returns a Base64 data URI string.
func toBase64DataURI(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading image file: %w", err)
	}

	mimeType := mimeTypeFromExt(filepath.Ext(path))
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded), nil
}

// mimeTypeFromExt returns the MIME type for common image extensions.
func mimeTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "image/png"
	}
}
