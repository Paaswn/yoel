package core

import (
	"context"
	"testing"
)

func TestNewQuestion(t *testing.T) {
    session, error := LoadSession()
    if error != nil {
        t.Fatal(error)
    }
    problem := ProblemLite{
        ID: 333,
        CodeName: "code-name",
        PrettyName: "Pretty Name",
    }
    ctx, cancel := context.WithTimeout(context.Background(), TimeOut)
    defer cancel()
    if err := CreateQuestion(ctx, session, problem ); err != nil {
        t.Fatal(err)
    }
}