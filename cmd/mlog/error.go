package main

import (
	"fmt"
	"github.com/urfave/cli/v2"

	"github.com/go-errors/errors"
)

// ErrorStacker allows us to check if the error can be displayed with more detail. It's
// intentionally satisfied by errors like github.com/go-errors/errors and CLIError.
type ErrorStacker interface {
	error
	ErrorStack() string
}

// Messager allows us to extract command messages via errors.As for
// printing on the command line.
type Messager interface {
	Message() string
}

// Prints the error object at a verbosity based on debug set/unset. Passes along an exit code if
// applicable. A cli.MultiError gets passed straight to cli.HandleExitCoder.
// Assumption is that other special printing needs (error state or not) will happen before getting
// to this handler.
func customErrorHandler(cCtx *cli.Context, err error) {
	if err == nil {
		cli.HandleExitCoder(err)
		return
	} else if multiErr := cli.MultiError(nil); errors.As(err, &multiErr) {
		cli.HandleExitCoder(multiErr)
		return
	}

	var message any = err
	if cCtx.Bool("debug") {
		if stErr := ErrorStacker(nil); errors.As(err, &stErr) {
			// Print the full error object in debug mode.
			message = stErr.ErrorStack()
		}
	} else {
		if cliErr := Messager(nil); errors.As(err, &cliErr) {
			// Print the simple version of the error object.
			message = cliErr.Message()
		}
	}
	code := 0
	if exitErr := cli.ExitCoder(nil); errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = 1
	}

	cli.HandleExitCoder(cli.Exit(message, code))
}

// TODO Make most output print using cli.ErrWriter (os.Stderr).

// CLIError implements cli.ExitCoder, uses go-errors.Error (stack trace)
// while providing a simplified version of the error message for printing on the
// command line.
// This setup allows the cli app error handler to decide whether to print msg
// or msg + full stack trace
type CLIError struct {
	error
	msg      string
	exitCode int
}

func (ce *CLIError) Unwrap() error {
	return ce.error
}

func (ce *CLIError) Message() string {
	return ce.msg
}

func (ce *CLIError) Error() string {
	if ce.error != nil {
		return fmt.Sprintf("%s\nExit code: %d\n%v", ce.msg, ce.exitCode, ce.error)
	}
	return fmt.Sprintf("%s\nExit code: %d", ce.msg, ce.exitCode)
}

func (ce *CLIError) ExitCode() int {
	return ce.exitCode
}

func WithStack(msg string) error {
	return newCLIError(nil, msg)
}

func WithStackF(format string, a ...any) error {
	return newCLIError(nil, fmt.Sprintf(format, a...))
}

func WrapWithStack(err error, msg string) error {
	return newCLIError(err, msg)
}

func WrapWithStackF(err error, format string, a ...any) error {
	return newCLIError(err, fmt.Sprintf(format, a...))
}

func newCLIError(err error, msg string) error {
	ce := &CLIError{error: err, msg: msg, exitCode: 1}
	// For the stack trace, skip this function AND the function calling this function.
	return errors.Wrap(ce, 2)
}
