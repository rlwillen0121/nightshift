package nightshift

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func readKinds(t *testing.T, payload []byte) (*byteReader, []keyKind) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	go func() {
		_, _ = writer.Write(payload)
		_ = writer.Close()
	}()
	var kinds []keyKind
	parser := &byteReader{f: reader}
	for {
		kind, err := parser.nextKey()
		if err != nil {
			kinds = append(kinds, keyEOF)
			return parser, kinds
		}
		kinds = append(kinds, kind)
		if kind == keyQuit {
			return parser, kinds
		}
	}
}

func TestKeyClassificationDoesNotRetainBytes(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    []keyKind
	}{
		{name: "letters", payload: []byte("ab"), want: []keyKind{keyTrigger, keyTrigger, keyEOF}},
		{name: "space", payload: []byte(" "), want: []keyKind{keyTrigger, keyEOF}},
		{name: "utf8", payload: []byte{0xc3, 0xa9}, want: []keyKind{keyTrigger, keyEOF}},
		{name: "enter", payload: []byte("\r\n"), want: []keyKind{keyEnter, keyEOF}},
		{name: "linefeed", payload: []byte("\n"), want: []keyKind{keyEnter, keyEOF}},
		{name: "esc-cr", payload: []byte{'q', 0x1b, '\r'}, want: []keyKind{keyTrigger, keyIgnore, keyEnter, keyEOF}},
		{name: "arrows", payload: []byte("\x1b[A\x1b[B\x1b[C\x1b[D\x1b[1;5A\x1bOA"), want: []keyKind{keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyEOF}},
		{name: "controls", payload: []byte{0x09, 0x7f, 0x08, 0x04, 0x1a, 0x1b}, want: []keyKind{keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyIgnore, keyEOF}},
		{name: "quit", payload: []byte{0x03}, want: []keyKind{keyQuit}},
		{name: "paste", payload: []byte("\x1b[200~SUPERSECRETINPUT\x03\x1b[201~"), want: []keyKind{keyTrigger, keyEOF}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			parser, got := readKinds(t, test.payload)
			if strings.Contains(fmt.Sprint(got), "SUPERSECRETINPUT") {
				t.Fatal(got)
			}
			if len(got) != len(test.want) {
				t.Fatalf("kinds = %v", got)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("kinds = %v", got)
				}
			}
			if test.name == "paste" {
				if bytes.Contains(parser.buf, []byte("SUPERSECRETINPUT")) || bytes.Contains(parser.scratch, []byte("SUPERSECRETINPUT")) {
					t.Fatalf("buf %q scratch %q", parser.buf, parser.scratch)
				}
			}
		})
	}

	secret := "SUPERSECRETINPUT"
	model := NewModel(Config{Seed: 3, SeedProvided: true})
	model, lines, quit := model.Handle(keyTrigger)
	if quit || len(lines) < burstMin {
		t.Fatal("paste trigger did not append a burst")
	}
	if strings.Contains(fmt.Sprintf("%#v\n%s", model, model.Transcript()), secret) {
		t.Fatal("paste bytes were retained")
	}
}
