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
	Start int
	End   int
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
	timestampRE = regexp.MustCompile(`\b(?:\d{4}-\d{2}-\d{2}T)?\d{2}:\d{2}:\d{2}(?:\.\d+)?Z\b`)
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
	add := func(class TokenClass, start, end int) {
		if start < 0 || end <= start || start >= len(text) || end > len(text) {
			return
		}
		tokens = append(tokens, Token{Class: class, Text: text[start:end], Start: start, End: end})
	}
	if loc := timestampRE.FindStringIndex(text); loc != nil {
		add(ClassTimestamp, loc[0], loc[1])
	}
	if index := strings.Index(text, "go test"); index >= 0 {
		add(ClassCommand, index, len(text))
	} else if index := strings.Index(text, "$ "); index >= 0 && strings.TrimSpace(text[:index]) == "" {
		add(ClassCommand, index, len(text))
	}
	if strings.HasPrefix(text, "ok") || strings.Contains(text, "--- PASS") || strings.TrimSpace(text) == "PASS" {
		index := strings.Index(text, "ok")
		if index < 0 {
			index = strings.Index(text, "PASS")
		}
		add(ClassOK, index, index+len(firstTokenAt(text, index)))
	} else if index := strings.Index(text, " ok "); index >= 0 {
		start := index + 1
		add(ClassOK, start, start+len(firstTokenAt(text, start)))
	}
	if strings.Contains(text, "##[warning]") || strings.Contains(text, "WARN") || strings.Contains(text, "--- SKIP") {
		add(ClassWarn, 0, len(text))
	}
	if isFailureText(text) {
		add(ClassFail, 0, len(text))
	}
	for _, loc := range pathRE.FindAllStringIndex(text, -1) {
		add(ClassPath, loc[0], loc[1])
	}
	for _, loc := range quotedRE.FindAllStringIndex(text, -1) {
		add(ClassString, loc[0], loc[1])
	}
	if strings.Contains(text, " want ") {
		index := strings.Index(text, " want ") + 1
		add(ClassWant, index, len(text))
	}
	if strings.Contains(text, "exit code") {
		loc := numberRE.FindAllStringIndex(text, -1)
		if len(loc) > 0 {
			last := loc[len(loc)-1]
			add(ClassNumber, last[0], last[1])
		}
	}
	return tokens
}

func firstTokenAt(text string, index int) string {
	if index < 0 || index >= len(text) {
		return ""
	}
	end := index
	for end < len(text) && !strings.ContainsRune(" \t", rune(text[end])) {
		end++
	}
	return text[index:end]
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
