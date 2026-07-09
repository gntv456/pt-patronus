package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

var (
	ErrUnsupportedTerminalTarget = errors.New("terminal is only supported for SSH hosts")
	ErrUnsupportedTerminalRunner = errors.New("collector does not support terminal")
)

// TerminalRunner is implemented by collectors that can open an interactive terminal.
type TerminalRunner interface {
	RunTerminal(ctx context.Context, t *models.MonitorTarget, creds Credentials, input io.Reader, output io.Writer, cols, rows int) error
}

func (c SSHCollector) RunTerminal(ctx context.Context, t *models.MonitorTarget, creds Credentials, input io.Reader, output io.Writer, cols, rows int) error {
	ex, err := c.dial(t, creds, sshTimeout(t))
	if err != nil {
		return err
	}
	defer ex.Close()
	term, ok := ex.(sshTerminalExecutor)
	if !ok {
		return ErrUnsupportedTerminalRunner
	}
	return term.Shell(ctx, input, output, cols, rows)
}

// RunTerminalNow opens an SSH terminal against a monitor target.
func (s *Service) RunTerminalNow(ctx context.Context, id uint, input io.Reader, output io.Writer, cols, rows int) error {
	t, err := s.store.Get(id)
	if err != nil {
		return err
	}
	protocol := ResolveProtocol(t)
	if t.Type != "host" || protocol != "ssh" {
		return fmt.Errorf("%w: %s/%s", ErrUnsupportedTerminalTarget, t.Type, protocol)
	}
	c, ok := s.reg.CollectorFor(t.Type, protocol)
	if !ok {
		return ErrUnsupported
	}
	runner, ok := c.(TerminalRunner)
	if !ok {
		return ErrUnsupportedTerminalRunner
	}
	creds, err := s.store.Credentials(t)
	if err != nil {
		return err
	}
	return runner.RunTerminal(ctx, t, creds, input, output, cols, rows)
}
