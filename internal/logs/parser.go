package logs

import (
	"bufio"
	"regexp"
	"sort"
	"strconv"
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
	Lines       []Line
	Folds       []Fold
	Commands    []Command
	Annotations []Annotation
	Masks       []string
	Failure     FailureWindow
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

type Command struct {
	Name       string
	Properties map[string]string
	Message    string
	Line       int
	Legacy     bool
}

type Annotation struct {
	Level       string
	Message     string
	Path        string
	Title       string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
	Line        int
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
	var commands []Command
	var annotations []Annotation
	var masks []string
	var stoppedCommandToken string

	for scanner.Scan() {
		text := strings.TrimPrefix(scanner.Text(), "\ufeff")
		lineNumber := len(lines) + 1
		commandText := commandSegment(text)

		if stoppedCommandToken != "" {
			if commandText == "::"+stoppedCommandToken+"::" {
				stoppedCommandToken = ""
			}
			visibleText := redact(text, masks)
			lines = append(lines, Line{
				Number: lineNumber,
				Text:   visibleText,
				Tokens: tokenize(visibleText),
			})
			continue
		}

		if command, ok := parseCommand(commandText, lineNumber); ok {
			if command.Name == "endgroup" {
				if len(stack) > 0 {
					fold := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					fold.EndLine = len(lines)
					fold.CollapsedCount = max(0, fold.EndLine-fold.StartLine)
					folds = append(folds, fold)
				}
				continue
			}

			if command.Name == "add-mask" {
				masks = addMasks(masks, command.Message)
				command.Message = "***"
			} else {
				command.Message = redact(command.Message, masks)
			}
			for key, value := range command.Properties {
				command.Properties[key] = redact(value, masks)
			}
			commands = append(commands, command)

			visibleText := redactCommandLine(text, commandText, command, masks)
			line := Line{
				Number: lineNumber,
				Text:   visibleText,
				Tokens: tokenize(visibleText),
			}
			lines = append(lines, line)

			switch command.Name {
			case "group":
				stack = append(stack, Fold{
					Title:     strings.TrimSpace(command.Message),
					StartLine: line.Number,
					Depth:     len(stack),
				})
			case "notice", "warning", "error":
				annotations = append(annotations, annotationFromCommand(command))
			case "stop-commands":
				stoppedCommandToken = command.Message
			}
			continue
		}

		if strings.HasPrefix(commandText, "##[endgroup]") {
			if len(stack) > 0 {
				fold := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				fold.EndLine = len(lines)
				fold.CollapsedCount = max(0, fold.EndLine-fold.StartLine)
				folds = append(folds, fold)
			}
			continue
		}

		visibleText := redact(text, masks)
		line := Line{
			Number: lineNumber,
			Text:   visibleText,
			Tokens: tokenize(visibleText),
		}
		lines = append(lines, line)

		if after, ok := strings.CutPrefix(commandText, "##[group]"); ok {
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
		Lines:       lines,
		Folds:       folds,
		Commands:    commands,
		Annotations: annotations,
		Masks:       masks,
		Failure:     extractFailure(lines),
	}
}

func commandSegment(text string) string {
	if loc := timestampRE.FindStringIndex(text); loc != nil && loc[0] == 0 && loc[1] < len(text) {
		return strings.TrimLeft(text[loc[1]:], " \t")
	}
	return text
}

func parseCommand(text string, lineNumber int) (Command, bool) {
	if command, ok := parseModernCommand(text, lineNumber); ok {
		return command, true
	}
	return parseLegacyCommand(text, lineNumber)
}

func parseModernCommand(text string, lineNumber int) (Command, bool) {
	if !strings.HasPrefix(text, "::") {
		return Command{}, false
	}
	end := strings.Index(text[2:], "::")
	if end < 0 {
		return Command{}, false
	}
	header := text[2 : 2+end]
	message := text[2+end+2:]
	name, properties := parseCommandHeader(header, ",")
	if name == "" {
		return Command{}, false
	}
	return Command{
		Name:       name,
		Properties: properties,
		Message:    decodeWorkflowData(message),
		Line:       lineNumber,
	}, true
}

func parseLegacyCommand(text string, lineNumber int) (Command, bool) {
	if !strings.HasPrefix(text, "##[") {
		return Command{}, false
	}
	end := strings.IndexByte(text, ']')
	if end < 0 {
		return Command{}, false
	}
	header := text[3:end]
	message := text[end+1:]
	name, properties := parseCommandHeader(header, ",;")
	if name == "" {
		return Command{}, false
	}
	return Command{
		Name:       name,
		Properties: properties,
		Message:    decodeWorkflowData(message),
		Line:       lineNumber,
		Legacy:     true,
	}, true
}

func parseCommandHeader(header, separators string) (string, map[string]string) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", nil
	}
	name := header
	propertiesText := ""
	if index := strings.IndexAny(header, " \t"); index >= 0 {
		name = header[:index]
		propertiesText = strings.TrimSpace(header[index+1:])
	}
	name = strings.ToLower(strings.TrimSpace(name))
	properties := make(map[string]string)
	for _, property := range splitAny(propertiesText, separators) {
		key, value, ok := strings.Cut(property, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		properties[key] = decodeWorkflowProperty(strings.TrimSpace(value))
	}
	return name, properties
}

func splitAny(text, separators string) []string {
	if text == "" {
		return nil
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(separators, r)
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func decodeWorkflowData(text string) string {
	return replaceEscapes(text, []string{"%0D", "\r", "%0A", "\n", "%25", "%"})
}

func decodeWorkflowProperty(text string) string {
	return replaceEscapes(text, []string{"%0D", "\r", "%0A", "\n", "%3A", ":", "%2C", ",", "%25", "%"})
}

func replaceEscapes(text string, pairs []string) string {
	for i := 0; i < len(pairs); i += 2 {
		text = replaceEscape(text, pairs[i], pairs[i+1])
	}
	return text
}

func replaceEscape(text, old, new string) string {
	text = strings.ReplaceAll(text, old, new)
	return strings.ReplaceAll(text, strings.ToLower(old), new)
}

func addMasks(masks []string, value string) []string {
	seen := make(map[string]struct{}, len(masks)+4)
	for _, mask := range masks {
		seen[mask] = struct{}{}
	}
	for _, candidate := range append([]string{value}, strings.Fields(value)...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		masks = append(masks, candidate)
	}
	sort.SliceStable(masks, func(i, j int) bool {
		return len(masks[i]) > len(masks[j])
	})
	return masks
}

func redactCommandLine(text, _ string, _ Command, masks []string) string {
	return redact(text, masks)
}

func redact(text string, masks []string) string {
	if text == "" || len(masks) == 0 {
		return text
	}
	for _, mask := range masks {
		if mask != "" {
			text = strings.ReplaceAll(text, mask, "***")
		}
	}
	return text
}

func annotationFromCommand(command Command) Annotation {
	return Annotation{
		Level:       command.Name,
		Message:     command.Message,
		Path:        property(command.Properties, "file", ".github"),
		Title:       property(command.Properties, "title", ""),
		StartLine:   propertyInt(command.Properties, "line", 1),
		EndLine:     propertyInt(command.Properties, "endline", 1),
		StartColumn: propertyInt(command.Properties, "col", 0),
		EndColumn:   propertyInt(command.Properties, "endcolumn", 0),
		Line:        command.Line,
	}
}

func property(properties map[string]string, key, fallback string) string {
	if properties == nil {
		return fallback
	}
	if value, ok := properties[strings.ToLower(key)]; ok && value != "" {
		return value
	}
	return fallback
}

func propertyInt(properties map[string]string, key string, fallback int) int {
	value := property(properties, key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
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
	if strings.Contains(text, "##[warning") || strings.Contains(text, "::warning") || strings.Contains(text, "WARN") || strings.Contains(text, "--- SKIP") {
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
	return strings.Contains(text, "##[error") ||
		strings.Contains(text, "::error") ||
		strings.Contains(text, "--- FAIL:") ||
		strings.HasPrefix(text, "FAIL ") ||
		strings.Contains(strings.ToLower(text), "error:")
}

func isFailureText(text string) bool {
	return strings.Contains(text, "##[error") ||
		strings.Contains(text, "::error") ||
		strings.Contains(text, "FAIL") ||
		strings.Contains(strings.ToLower(text), "error:")
}
