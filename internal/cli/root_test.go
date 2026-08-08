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

func TestNewRootCommandIncludesLoginCommand(t *testing.T) {
	root := NewRootCommand()
	login, _, err := root.Find([]string{"login"})
	if err != nil {
		t.Fatalf("Find(login) error = %v", err)
	}
	if login == root {
		t.Fatal("Find(login) returned the root command")
	}
	if login.Use != "login" {
		t.Fatalf("login.Use = %q, want %q", login.Use, "login")
	}
}
