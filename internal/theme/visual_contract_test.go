package theme

import (
	"os"
	"strings"
	"testing"
)

func TestThemeTokensMatchVisualContract(t *testing.T) {
	contract := readVisualContract(t, "../../docs/visual-contract.md")
	expected := map[string]string{
		"bramble bg":        ForMode(ModeBramble).BG,
		"bramble bg-elev":   ForMode(ModeBramble).BGElev,
		"bramble surface":   ForMode(ModeBramble).Surface,
		"bramble surface-2": ForMode(ModeBramble).Surface2,
		"bramble line":      ForMode(ModeBramble).Line,
		"bramble line-2":    ForMode(ModeBramble).Line2,
		"bramble dim":       ForMode(ModeBramble).Dim,
		"bramble subtle":    ForMode(ModeBramble).Subtle,
		"bramble muted":     ForMode(ModeBramble).Muted,
		"bramble fg-soft":   ForMode(ModeBramble).FGSoft,
		"bramble fg":        ForMode(ModeBramble).FG,
		"bramble ok":        ForMode(ModeBramble).OK,
		"bramble ok-deep":   ForMode(ModeBramble).OKDeep,
		"bramble fail":      ForMode(ModeBramble).Fail,
		"bramble run":       ForMode(ModeBramble).Run,
		"bramble info":      ForMode(ModeBramble).Info,
		"bramble warn":      ForMode(ModeBramble).Warn,
		"bramble neutral":   ForMode(ModeBramble).Neutral,
		"bramble term-bg":   ForMode(ModeBramble).TermBG,
		"bone bg":           ForMode(ModeBone).BG,
		"bone fg":           ForMode(ModeBone).FG,
		"bone ok":           ForMode(ModeBone).OK,
		"bone fail":         ForMode(ModeBone).Fail,
		"bone run":          ForMode(ModeBone).Run,
		"bone info":         ForMode(ModeBone).Info,
		"bone warn":         ForMode(ModeBone).Warn,
		"bone neutral":      ForMode(ModeBone).Neutral,
		"bone term-bg":      ForMode(ModeBone).TermBG,
	}

	for key, value := range expected {
		themeName, token, ok := strings.Cut(key, " ")
		if !ok {
			t.Fatalf("bad test key %q", key)
		}
		row := "| " + themeName + " | " + token + " | " + value + " |"
		if !strings.Contains(contract, row) {
			t.Fatalf("visual contract missing token row %q", row)
		}
	}
}

func readVisualContract(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read visual contract: %v", err)
	}
	return string(data)
}
