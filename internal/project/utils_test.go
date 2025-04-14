package project

import (
	"context"
	"testing"
)

func TestGoMod(t *testing.T) {
	if err := GoMod(context.Background(), "tidy"); err != nil {
		t.Fatal(err)
	}
}

func TestGoGet(t *testing.T) {
	if err := GoGet(context.Background(), "github.com/spf13/cobra"); err != nil {
		t.Fatal(err)
	}
}
