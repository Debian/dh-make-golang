package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/russross/blackfriday"
)

type readmeReply struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	Name     string `json:"name"`
}

func getLongDescriptionForGopkg(gopkg string) (string, error) {
	if !strings.HasPrefix(gopkg, "github.com/") {
		return "", nil
	}
	resp, err := http.Get("https://api.github.com/repos/" + gopkg[len("github.com/"):] + "/readme")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var rr readmeReply
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return "", err
	}

	if rr.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding: got %q, want %q", rr.Encoding, "base64")
	}

	content, err := base64.StdEncoding.DecodeString(rr.Content)
	if err != nil {
		return "", err
	}

	// Supported filename suffixes are from
	// https://github.com/github/markup/blob/master/README.md
	// NOTE(stapelberg): Ideally, we’d use https://github.com/github/markup
	// itself to render to HTML, then convert HTML to plaintext. That sounds
	// fairly involved, but it’d be the most correct solution to the problem at
	// hand. Our current code just knows markdown, which is good enough since
	// most (Go?) projects in fact use markdown for their README files.
	if !strings.HasSuffix(rr.Name, "md") &&
		!strings.HasSuffix(rr.Name, "markdown") &&
		!strings.HasSuffix(rr.Name, "mdown") &&
		!strings.HasSuffix(rr.Name, "mkdn") {
		return strings.TrimSpace(string(content)), nil
	}

	output := blackfriday.Markdown(content, &TextRenderer{}, 0)
	// Shell out to fmt(1) to line-wrap the output.
	cmd := exec.Command("fmt")
	cmd.Stdin = bytes.NewBuffer(output)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type TextRenderer struct {
}

func (options *TextRenderer) BlockCode(out *bytes.Buffer, text []byte, lang string) {
	out.Write(text)
}

func (options *TextRenderer) BlockQuote(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) BlockHtml(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) Header(out *bytes.Buffer, text func() bool, level int, id string) {
	text()
}

func (options *TextRenderer) HRule(out *bytes.Buffer) {
	out.WriteString("--------------------------------------------------------------------------------\n")
}

func (options *TextRenderer) List(out *bytes.Buffer, text func() bool, flags int) {
	text()
}

func (options *TextRenderer) ListItem(out *bytes.Buffer, text []byte, flags int) {
	out.WriteString("• ")
	out.Write(text)
}

func (options *TextRenderer) Paragraph(out *bytes.Buffer, text func() bool) {
	out.WriteString("\n")
	text()
	out.WriteString("\n")
}

func (options *TextRenderer) Table(out *bytes.Buffer, header []byte, body []byte, columnData []int) {
	out.Write(header)
	out.Write(body)
}

func (options *TextRenderer) TableRow(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) TableHeaderCell(out *bytes.Buffer, text []byte, flags int) {
	out.Write(text)
}

func (options *TextRenderer) TableCell(out *bytes.Buffer, text []byte, flags int) {
	out.Write(text)
}

func (options *TextRenderer) Footnotes(out *bytes.Buffer, text func() bool) {
	text()
}

func (options *TextRenderer) FootnoteItem(out *bytes.Buffer, name, text []byte, flags int) {
	out.WriteString("[")
	out.Write(name)
	out.WriteString("]")
	out.Write(text)
}

func (options *TextRenderer) TitleBlock(out *bytes.Buffer, text []byte) {
}

// Span-level callbacks
func (options *TextRenderer) AutoLink(out *bytes.Buffer, link []byte, kind int) {
	out.Write(link)
}

func (options *TextRenderer) CodeSpan(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) DoubleEmphasis(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) Emphasis(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) Image(out *bytes.Buffer, link []byte, title []byte, alt []byte) {
	out.Write(alt)
}

func (options *TextRenderer) LineBreak(out *bytes.Buffer) {
	out.WriteString("\n")
}

func (options *TextRenderer) Link(out *bytes.Buffer, link []byte, title []byte, content []byte) {
	out.Write(content)
	out.WriteString(" (")
	out.Write(link)
	out.WriteString(")")
}

func (options *TextRenderer) RawHtmlTag(out *bytes.Buffer, tag []byte) {
}

func (options *TextRenderer) TripleEmphasis(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) StrikeThrough(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

func (options *TextRenderer) FootnoteRef(out *bytes.Buffer, ref []byte, id int) {
}

// Low-level callbacks
func (options *TextRenderer) Entity(out *bytes.Buffer, entity []byte) {
	out.Write(entity)
}

func (options *TextRenderer) NormalText(out *bytes.Buffer, text []byte) {
	out.Write(text)
}

// Header and footer
func (options *TextRenderer) DocumentHeader(out *bytes.Buffer) {
}

func (options *TextRenderer) DocumentFooter(out *bytes.Buffer) {
}

func (options *TextRenderer) GetFlags() int {
	return 0
}
