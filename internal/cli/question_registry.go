package cli

import (
	"fmt"

	"yoel/internal/graderapi"
	"yoel/internal/registry"
)

func refreshQuestionRegistry(problems []graderapi.Problem) error {
	questions, err := registry.LoadDefault()
	if err != nil {
		return err
	}
	for _, problem := range problems {
		if err := questions.UpsertRemote(problem.ID, problem.Name, problem.FullName); err != nil {
			return err
		}
	}
	path, err := registry.DefaultPath()
	if err != nil {
		return err
	}
	return questions.Save(path)
}

func recordCreatedQuestion(problem graderapi.Problem, directoryPath, sourcePath string) error {
	questions, err := registry.LoadDefault()
	if err != nil {
		return err
	}
	if err := questions.UpsertRemote(problem.ID, problem.Name, problem.FullName); err != nil {
		return err
	}
	if err := questions.BindLocal(problem.ID, directoryPath, sourcePath); err != nil {
		return err
	}
	path, err := registry.DefaultPath()
	if err != nil {
		return err
	}
	if err := questions.Save(path); err != nil {
		return err
	}
	return nil
}

func recordCreatedQuestionError(problemID int, err error) error {
	return fmt.Errorf("question %d was created but could not be recorded in the question registry: %w", problemID, err)
}
