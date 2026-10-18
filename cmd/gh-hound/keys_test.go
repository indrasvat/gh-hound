package main

import (
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
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
// key is valid UTF-8, one named key or one rune, and text without
// control bytes comes back exactly as typed however the reads split.
func FuzzKeyDecoder(f *testing.F) {
	for _, seed := range []string{"café ✓", "\U0001F415j\x1b[A", "\xc3", "\xe2\x9c", "é\x1b", "\xff\xfe"} {
		f.Add([]byte(seed), uint8(1))
		f.Add([]byte(seed), uint8(3))
	}
	f.Fuzz(func(t *testing.T, data []byte, size uint8) {
		keys := decodeAll(t, &chunkReader{data: slices.Clone(data), size: int(size%8) + 1})
		var typed strings.Builder
		for _, key := range keys {
			if !utf8.ValidString(key) || key == "" {
				t.Fatalf("%q: key %q is not one valid rune or name", data, key)
			}
			if key == "space" {
				typed.WriteByte(' ')
			} else {
				typed.WriteString(key)
			}
		}
		if !utf8.Valid(data) || strings.ContainsFunc(string(data), func(r rune) bool { return r < 0x20 || r == 0x7f }) {
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
