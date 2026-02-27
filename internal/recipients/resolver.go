package recipients

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/MihkelHunter/release-notifier/internal/config"
)

// Recipient is a single email recipient.
type Recipient struct {
	Name  string
	Email string
}

// CSV format:
//   email, name, tags
//   john.doe@company.com, John Doe, "backend,ops"
//   jane.smith@company.com, Jane Smith, finance
//   cto@company.com, CTO Office, "all,finance,backend"
//
// Special tag "all" always matches.

// Resolve reads the CSV and returns recipients whose tags overlap with the
// provided tags list. It also adds recipients for always_include_tags from
// the environment config.
func Resolve(csvPath string, noteTags []string, env string, cfg *config.Config) ([]Recipient, error) {
	// Merge note tags with always_include_tags for the environment
	allTags := make(map[string]struct{})
	for _, t := range noteTags {
		allTags[strings.ToLower(t)] = struct{}{}
	}
	if envCfg, ok := cfg.Environments[env]; ok {
		for _, t := range envCfg.AlwaysIncludeTags {
			allTags[strings.ToLower(t)] = struct{}{}
		}
	}

	records, err := readCSV(csvPath)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var result []Recipient

	for _, rec := range records {
		if matchesTags(rec.tags, allTags) {
			emailLower := strings.ToLower(rec.email)
			if _, exists := seen[emailLower]; !exists {
				seen[emailLower] = struct{}{}
				result = append(result, Recipient{
					Name:  rec.name,
					Email: rec.email,
				})
			}
		}
	}

	return result, nil
}

// csvRow is an internal representation of one CSV row.
type csvRow struct {
	email string
	name  string
	tags  []string
}

func readCSV(path string) ([]csvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening recipients CSV %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comment = '#'
	r.TrimLeadingSpace = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing CSV: %w", err)
	}

	var result []csvRow
	for i, row := range rows {
		// Skip header row
		if i == 0 && (strings.ToLower(strings.TrimSpace(row[0])) == "email") {
			continue
		}
		if len(row) < 3 {
			continue
		}

		var tags []string
		for _, t := range strings.Split(row[2], ",") {
			tag := strings.ToLower(strings.TrimSpace(t))
			if tag != "" {
				tags = append(tags, tag)
			}
		}

		result = append(result, csvRow{
			email: strings.TrimSpace(row[0]),
			name:  strings.TrimSpace(row[1]),
			tags:  tags,
		})
	}

	return result, nil
}

// matchesTags returns true if any of the recipient's tags match the active tag set,
// or if the recipient has the special "all" tag.
func matchesTags(recipientTags []string, activeTags map[string]struct{}) bool {
	for _, rt := range recipientTags {
		if rt == "all" {
			return true
		}
		if _, ok := activeTags[rt]; ok {
			return true
		}
	}
	return false
}
