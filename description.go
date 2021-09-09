package main

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

//go:embed description.json
var descriptionJSONBytes []byte

// reformatForControl reformats the wrapped description
// to conform to Debian’s control format.
func reformatForControl(raw string) string {
	output := ""
	next_prefix := ""
	re := regexp.MustCompile(`^ \d+\. `)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		// Remove paddings that Glamour currently add to the end of each line
		line = strings.TrimRight(line, " ")

		prefix := next_prefix
		if strings.HasPrefix(line, " * ") {
			prefix = ""
			next_prefix = "  "
		}
		if re.MatchString(line) {
			prefix = ""
			next_prefix = "   "
		}
		if line == "" {
			line = "."
			prefix = ""
			next_prefix = ""
		}
		output += " " + prefix + line + "\n"
	}
	return output
}

// getDescriptionForGopkg reads from README.md (or equivalent) from GitHub,
// intended for the extended description in debian/control.
func getLongDescriptionForGopkg(gopkg string) (string, error) {
	owner, repo, err := findGitHubRepo(gopkg)
	if err != nil {
		return "", fmt.Errorf("find github repo: %w", err)
	}

	rr, _, err := gitHub.Repositories.GetReadme(context.TODO(), owner, repo, nil)
	if err != nil {
		return "", fmt.Errorf("get readme: %w", err)
	}

	content, err := rr.GetContent()
	if err != nil {
		return "", fmt.Errorf("get content: %w", err)
	}

	// Supported filename suffixes are from
	// https://github.com/github/markup/blob/master/README.md
	// NOTE(stapelberg): Ideally, we’d use https://github.com/github/markup
	// itself to render to HTML, then convert HTML to plaintext. That sounds
	// fairly involved, but it’d be the most correct solution to the problem at
	// hand. Our current code just knows markdown, which is good enough since
	// most (Go?) projects in fact use markdown for their README files.
	if !strings.HasSuffix(rr.GetName(), "md") &&
		!strings.HasSuffix(rr.GetName(), "markdown") &&
		!strings.HasSuffix(rr.GetName(), "mdown") &&
		!strings.HasSuffix(rr.GetName(), "mkdn") {
		return reformatForControl(strings.TrimSpace(string(content))), nil
	}

	r, _ := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(descriptionJSONBytes),
		// wrap output at specific width
		glamour.WithWordWrap(72),
	)

	out, err := r.Render(content)
	if err != nil {
		return "", fmt.Errorf("fmt: %w", err)
	}
	return reformatForControl(strings.TrimSpace(string(out))), nil
}
