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

		// Try to add hanging indent for list items that span over one line
		prefix := next_prefix
		if strings.HasPrefix(line, " * ") {
			// unordered list
			prefix = ""
			next_prefix = "  "
		}
		if re.MatchString(line) {
			// ordered list
			prefix = ""
			next_prefix = "   "
		}
		if line == "" {
			// blank line, implying end of list
			line = "."
			prefix = ""
			next_prefix = ""
		}
		output += " " + prefix + line + "\n"
	}
	return output
}

// markdownToLongDescription converts Markdown to plain text
// and reformat it for expanded description in debian/control.
func markdownToLongDescription(markdown string) (string, error) {
	r, _ := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(descriptionJSONBytes),
		glamour.WithWordWrap(72),
	)
	out, err := r.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("fail to render Markdown: %w", err)
	}
	//fmt.Println(out)
	//fmt.Println(reformatForControl(out))
	return reformatForControl(out), nil
}

// getDescriptionForGopkg reads from README.md (or equivalent) from GitHub,
// intended for extended description in debian/control.
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
		return reformatForControl(content), nil
	}

	return markdownToLongDescription(content)
}
