package media

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

var errOutputLimit = errors.New("command output limit exceeded")

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

type CommandRunner interface {
	Run(context.Context, string, []string, int64, int64) (CommandResult, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, executable string, args []string, maxStdout, maxStderr int64) (CommandResult, error) {
	stdout := newLimitedBuffer(maxStdout)
	stderr := newLimitedBuffer(maxStderr)
	command := exec.CommandContext(ctx, executable, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if stdout.Overflowed() || stderr.Overflowed() {
		return result, errOutputLimit
	}
	return result, err
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int64
	written  int64
	overflow bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	if limit <= 0 {
		limit = 1
	}
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(raw []byte) (int, error) {
	original := len(raw)
	remaining := b.limit - b.written
	if remaining <= 0 {
		b.overflow = true
		return original, nil
	}
	if int64(len(raw)) > remaining {
		raw = raw[:remaining]
		b.overflow = true
	}
	n, _ := b.buffer.Write(raw)
	b.written += int64(n)
	return original, nil
}

func (b *limitedBuffer) Bytes() []byte    { return append([]byte{}, b.buffer.Bytes()...) }
func (b *limitedBuffer) Overflowed() bool { return b.overflow }
