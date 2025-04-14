package project

import (
	"bytes"
	"errors"
	"github.com/spf13/afero"
	"strings"
	"unicode"
)

func compareContent(contentA, contentB []byte) error {
	if !bytes.Equal(ensureLF(contentA), ensureLF(contentB)) {
		return errors.New("byte slices differ")
	}
	return nil
}

func validateCmdName(args []string) string {
	var source string
	if len(args) > 0 {
		source = args[0]
	}

	var sb strings.Builder
	capitalize := false

	for i := 0; i < len(source); i++ {
		ch := source[i]
		if ch == '-' || ch == '_' {
			capitalize = true
			continue
		}
		if capitalize {
			sb.WriteByte(byte(unicode.ToUpper(rune(ch))))
			capitalize = false
		} else {
			sb.WriteByte(ch)
		}
	}

	if sb.Len() == 0 {
		return source
	}
	return sb.String()
}

// ensureLF converts any \r\n to \n
func ensureLF(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

func stat(afs afero.Fs, namePath string) bool {
	if _, err := afs.Stat(namePath); err != nil {
		return false
	}
	return true
}
