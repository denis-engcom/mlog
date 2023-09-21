package main

import (
	"fmt"

	// "github.com/carlmjohnson/exitcode"
	"github.com/go-errors/errors"
	// "github.com/urfave/cli/v2"
)

// TODO apply new error handling everywhere

// CommandMessager allows us to extract command messages via errors.As for
// printing on the command line.
type CommandMessager interface {
	error
	Message() string
}

// CommandMessage implements cli.ExitCoder, uses go-errors.Error (stack trace)
// while providing a simplified version of the error message for printing on the
// command line.
// This setup allows the cli app error handler to decide whether to print msg
// or msg + full stack trace
type CommandMessage struct {
	error
	msg      string
	exitCode int
}

func (cm CommandMessage) Unwrap() error {
	return cm.error
}

func (cm CommandMessage) Message() string {
	return cm.msg
}

func (cm CommandMessage) Error() string {
	if cm.error != nil {
		return fmt.Sprintf("%s, exitCode=%d: %v", cm.msg, cm.exitCode, cm.error)
	}
	return fmt.Sprintf("%s, exitCode=%d", cm.msg, cm.exitCode)
}

func (cm CommandMessage) ExitCode() int {
	return cm.exitCode
}

func WithStack(msg string) error {
	return NewCommandMessage(nil, msg)
}

func WrapWithStack(err error, msg string) error {
	return NewCommandMessage(err, msg)
}

func NewCommandMessage(err error, msg string) error {
	cm := CommandMessage{error: err, msg: msg, exitCode: 1}
	return errors.Wrap(cm, 2)
}