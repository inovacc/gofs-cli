package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/spf13/afero"
	"os/exec"
	"strings"
	"unicode"
)

var goBinary = "go"

// GoGet runs `go get -u <mod>` to fetch and update a module.
func GoGet(ctx context.Context, mod string) error {
	_, err := runGoCommand(ctx, []string{"get", "-u", mod})
	return err
}

// GoMod executes `go mod <args>` commands.
func GoMod(ctx context.Context, args ...string) error {
	allArgs := append([]string{"mod"}, args...)
	_, err := runGoCommand(ctx, allArgs)
	return err
}

// modInfoJSON unmarshal the output of `go list -json <args>` into the provided struct.
func modInfoJSON(v any, args ...string) error {
	allArgs := append([]string{"list", "-json"}, args...)
	out, err := runGoCommand(context.Background(), allArgs)
	if err != nil {
		return err
	}
	return json.Unmarshal(out, v)
}

// runGoCommand constructs and executes a Go command, returning its output.
func runGoCommand(ctx context.Context, args []string) ([]byte, error) {
	cmdArgs := append([]string{goBinary}, args...)
	//log.Printf("Running command: %v\n", cmdArgs)

	out, err := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
	if err != nil {
		return nil, errors.New(string(out))
	}

	return out, nil
}

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
