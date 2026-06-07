package logs

import (
	"bufio"
	"regexp"
	"sort"
	"strings"
)

type TokenClass string

const (
	ClassTimestamp TokenClass = "timestamp"
	ClassCommand   TokenClass = "command"
	ClassOK        TokenClass = "ok"
	ClassFail      TokenClass = "fail"
	ClassWarn      TokenClass = "warn"
	ClassPath      TokenClass = "path"
	ClassString    TokenClass = "string"
	ClassWant      TokenClass = "want"
	ClassNumber    TokenClass = "number"
)

type Document struct {
	Lines   []Line
	Folds   []Fold
	Failure FailureWindow
}

type Line struct {
	Number int
	Text   string
	Tokens []Token
}

type Token struct {
	Class TokenClass
	Text  string
}

type Fold struct {
	Title          string
	StartLine      int
	EndLine        int
	Depth          int
	CollapsedCount int
}

type FailureWindow struct {
	Found      bool
	AnchorLine int
	StartLine  int
	EndLine    int
	Lines      []Line
}

var (
	timestampRE = regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\b`)
	pathRE      = regexp.MustCompile(`\b[\w./-]+\.go:\d+\b`)
	quotedRE    = regexp.MustCompile(`"[^"]*"`)
	numberRE    = regexp.MustCompile(`\b\d+\b`)
)

func Parse(raw string) Document {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []Line
	var folds []Fold
	var stack []Fold

	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "##[endgroup]") {
			if len(stack) > 0 {
				fold := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				fold.EndLine = len(lines)
				fold.CollapsedCount = max(0, fold.EndLine-fold.StartLine)
				folds = append(folds, fold)
			}
			continue
		}

		line := Line{
			Number: len(lines) + 1,
			Text:   text,
			Tokens: tokenize(text),
		}
		lines = append(lines, line)

		if after, ok := strings.CutPrefix(text, "##[group]"); ok {
			stack = append(stack, Fold{
				Title:     strings.TrimSpace(after),
				StartLine: line.Number,
				Depth:     len(stack),
			})
		}
	}
	for len(stack) > 0 {
		fold := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		fold.EndLine = len(lines)
		fold.CollapsedCount = max(0, fold.EndLine-fold.StartLine)
		folds = append(folds, fold)
	}
	sort.SliceStable(folds, func(i, j int) bool {
		return folds[i].StartLine < folds[j].StartLine
	})

	return Document{
		Lines:   lines,
		Folds:   folds,
		Failure: extractFailure(lines),
	}
}

func tokenize(text string) []Token {
	tokens := make([]Token, 0, 4)
	if match := timestampRE.FindString(text); match != "" {
		tokens = append(tokens, Token{Class: ClassTimestamp, Text: match})
	}
	if strings.Contains(text, "go test") || strings.HasPrefix(strings.TrimSpace(text), "$ ") {
		tokens = append(tokens, Token{Class: ClassCommand, Text: strings.TrimSpace(text)})
	}
	if strings.HasPrefix(text, "ok") || strings.Contains(text, "--- PASS") || strings.TrimSpace(text) == "PASS" {
		tokens = append(tokens, Token{Class: ClassOK, Text: "ok"})
	}
	if strings.Contains(text, "##[warning]") || strings.Contains(text, "WARN") || strings.Contains(text, "--- SKIP") {
		tokens = append(tokens, Token{Class: ClassWarn, Text: "warn"})
	}
	if isFailureText(text) {
		tokens = append(tokens, Token{Class: ClassFail, Text: "fail"})
	}
	for _, match := range pathRE.FindAllString(text, -1) {
		tokens = append(tokens, Token{Class: ClassPath, Text: match})
	}
	for _, match := range quotedRE.FindAllString(text, -1) {
		tokens = append(tokens, Token{Class: ClassString, Text: match})
	}
	if strings.Contains(text, " want ") {
		tokens = append(tokens, Token{Class: ClassWant, Text: strings.TrimSpace(text[strings.Index(text, " want ")+1:])})
	}
	if strings.Contains(text, "exit code") {
		if match := numberRE.FindString(text); match != "" {
			tokens = append(tokens, Token{Class: ClassNumber, Text: match})
		}
	}
	return tokens
}

func extractFailure(lines []Line) FailureWindow {
	for i, line := range lines {
		if isFailureAnchor(line.Text) {
			start := max(0, i-3)
			end := min(len(lines), i+6)
			return FailureWindow{
				Found:      true,
				AnchorLine: line.Number,
				StartLine:  lines[start].Number,
				EndLine:    lines[end-1].Number,
				Lines:      append([]Line(nil), lines[start:end]...),
			}
		}
	}
	return FailureWindow{}
}

func isFailureAnchor(text string) bool {
	if pathRE.MatchString(text) && strings.Contains(text, " got ") && strings.Contains(text, " want ") {
		return true
	}
	return strings.Contains(text, "##[error]") ||
		strings.Contains(text, "--- FAIL:") ||
		strings.HasPrefix(text, "FAIL ") ||
		strings.Contains(strings.ToLower(text), "error:")
}

func isFailureText(text string) bool {
	return strings.Contains(text, "##[error]") ||
		strings.Contains(text, "FAIL") ||
		strings.Contains(strings.ToLower(text), "error:")
}
