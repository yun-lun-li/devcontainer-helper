package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

type synchronizedWriter struct {
	mu     *sync.Mutex
	writer io.Writer
}

func (writer *synchronizedWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.writer.Write(data)
}

func runParallel(ctx context.Context, workspace string, commands map[string]any, env []string, stdout, stderr io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Parallel hooks have no shared terminal input; use a single hook for prompts.
	var outputMutex sync.Mutex
	output := &synchronizedWriter{mu: &outputMutex, writer: stdout}
	errorOutput := &synchronizedWriter{mu: &outputMutex, writer: stderr}
	results := make(chan error, len(commands))
	for name, value := range commands {
		switch value.(type) {
		case string, []any:
		default:
			return fmt.Errorf("parallel hook %q must be a string or argument array", name)
		}
	}
	for name, value := range commands {
		go func() {
			raw, err := json.Marshal(value)
			if err == nil {
				err = runHook(ctx, workspace, raw, env, nil, output, errorOutput)
			}
			if err != nil {
				cancel()
				err = fmt.Errorf("%s: %w", name, err)
			}
			results <- err
		}()
	}
	var errs []error
	for range commands {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
