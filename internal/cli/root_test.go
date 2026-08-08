package cli

import (
	"bytes"
	"testing"
)

func TestNewRootCommandShowsHelpWithoutSubcommands(t *testing.T) {
	command := NewRootCommand()
	output := new(bytes.Buffer)
	command.SetOut(output)
	command.SetErr(output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("Usage:")) {
		t.Fatalf("help output = %q, want Usage", output.String())
	}
}
