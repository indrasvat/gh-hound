package main

import (
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"unicode"
	"unicode/utf8"
)

// chunkReader hands out at most size bytes per Read, like a terminal
// delivering one keystroke's bytes across several reads.
type chunkReader struct {
	data []byte
	size int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), r.size, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func decodeAll(t testing.TB, reader io.Reader) []string {
	t.Helper()
	decoder := keyDecoder{}
	scratch := make([]byte, 8)
	var keys []string
	for {
		key, err := decoder.Next(reader, scratch)
		if err != nil {
			return keys
		}
		keys = append(keys, key)
	}
}

func TestKeyNameDecodesUTF8(t *testing.T) {
	for _, input := range []string{"é", "✓", "世", "\U0001F415"} {
		if got := keyName([]byte(input)); got != input {
			t.Fatalf("keyName(%q) = %q, want %q", input, got, input)
		}
	}
}

func TestKeyDecoderBuffersRuneSplitAcrossReads(t *testing.T) {
	want := []string{"c", "a", "f", "é", "space", "✓", "\U0001F415"}
	for _, reader := range []io.Reader{
		strings.NewReader("café ✓\U0001F415"),
		iotest.OneByteReader(strings.NewReader("café ✓\U0001F415")),
		&chunkReader{data: []byte("café ✓\U0001F415"), size: 3},
	} {
		if got := decodeAll(t, reader); !slices.Equal(got, want) {
			t.Fatalf("keys = %q, want %q", got, want)
		}
	}
}

func TestKeyDecoderDropsInvalidUTF8(t *testing.T) {
	got := decodeAll(t, strings.NewReader("\xffj\xc3k\x80"))
	if want := []string{"j", "k"}; !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

// FuzzKeyDecoder feeds arbitrary bytes in arbitrary read sizes: every
// key is one named key or one printable rune, and text without
// control characters comes back exactly as typed however the reads split.
func FuzzKeyDecoder(f *testing.F) {
	for _, seed := range []string{"café ✓", "\U0001F415j\x1b[A", "\xc3", "\xe2\x9c", "é\x1b", "\xff\xfe", "a\x1b[3~b", "\x1b[1;5C\x1bOP"} {
		f.Add([]byte(seed), uint8(1))
		f.Add([]byte(seed), uint8(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, size uint8) {
		keys := decodeAll(t, &chunkReader{data: slices.Clone(data), size: int(size%8) + 1})
		var typed strings.Builder
		for _, key := range keys {
			if !utf8.ValidString(key) || key == "" || strings.ContainsFunc(key, unicode.IsControl) {
				t.Fatalf("%q: key %q is not one printable rune or name", data, key)
			}
			if key == "space" {
				typed.WriteByte(' ')
			} else {
				typed.WriteString(key)
			}
		}
		unbound := func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) }
		if !utf8.Valid(data) || strings.ContainsFunc(string(data), unbound) {
			return
		}
		if typed.String() != string(data) {
			t.Fatalf("typed %q, decoded %q", data, typed.String())
		}
		if len(keys) != utf8.RuneCount(data) {
			t.Fatalf("%q: %d keys for %d runes", data, len(keys), utf8.RuneCount(data))
		}
	})
}

// Keys the TUI doesn't bind (Delete, Home, End, F-keys, PageUp) send
// whole escape sequences: they must vanish, not land as esc + text.
func TestKeyDecoderSwallowsUnboundEscapeSequences(t *testing.T) {
	input := "a\x1b[3~b\x1b[Hc\x1bOPd\x1b[5~e\x1b[1;5Cf\x1bOA\x1b[Z\x1b"
	want := []string{"a", "b", "c", "d", "e", "f", "up", "shift+tab", "esc"}
	if got := decodeAll(t, strings.NewReader(input)); !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

// Unbound control characters (ctrl+L, BEL, NUL, 8-bit CSI) must never
// reach an input, which would write them to the terminal.
func TestKeyDecoderDropsUnboundControlRunes(t *testing.T) {
	got := decodeAll(t, strings.NewReader("a\x0c\x0b\x0e\x07\x00\xc2\x9b\xc2\x85b\x03"))
	if want := []string{"a", "b", "ctrl+c"}; !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

// A burst of arrows fills the read buffer and can end on a lone ESC:
// the sequence's rest is still to come, so it is not an esc key.
func TestKeyDecoderKeepsAnArrowSplitAtAFullRead(t *testing.T) {
	burst := strings.Repeat("\x1b[B", 6)
	want := slices.Repeat([]string{"down"}, 6)
	if got := decodeAll(t, &chunkReader{data: []byte(burst), size: 8}); !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

// Bidi controls reorder the line on terminals that honour them.
func TestKeyDecoderDropsBidiControls(t *testing.T) {
	got := decodeAll(t, strings.NewReader("a\u202eb\u2066c\u200dd"))
	if want := []string{"a", "b", "c", "\u200d", "d"}; !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
}

// An esc at the end of a read is a keypress, however full the read was
// (a paste, then esc): nothing following it soon means nothing will.
func TestReadKeysDeliversATrailingEscWithoutWaiting(t *testing.T) {
	stdin, keys := io.Pipe()
	defer func() { _ = keys.Close() }()
	events := readKeys(stdin)
	paste := strings.Repeat("x", 255)
	go func() { _, _ = io.WriteString(keys, paste+"\x1b") }()
	for _, want := range append(strings.Split(paste, ""), "esc") {
		select {
		case event := <-events:
			if event.key != want {
				t.Fatalf("key = %q, want %q", event.key, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("%q never arrived", want)
		}
	}
}

// An escape sequence split across writes (a full read, or SSH) is one
// key when its rest arrives inside the escape window.
func TestReadKeysJoinsASequenceSplitAcrossWrites(t *testing.T) {
	stdin, keys := io.Pipe()
	defer func() { _ = keys.Close() }()
	events := readKeys(stdin)
	go func() {
		_, _ = io.WriteString(keys, "\x1b")
		time.Sleep(escDelay / 5)
		_, _ = io.WriteString(keys, "[Bj")
	}()
	for _, want := range []string{"down", "j"} {
		select {
		case event := <-events:
			if event.key != want {
				t.Fatalf("key = %q, want %q", event.key, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("%q never arrived", want)
		}
	}
}
