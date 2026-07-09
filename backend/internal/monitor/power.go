package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

const (
	PowerActionReboot   = "reboot"
	PowerActionShutdown = "shutdown"
)

var ErrUnsupportedPowerAction = errors.New("unsupported power action")

// PowerController is implemented by collectors that can perform fixed power actions.
type PowerController interface {
	Power(ctx context.Context, t *models.MonitorTarget, creds Credentials, action string) error
}

// PowerNow triggers a power action on a monitor target.
func (s *Service) PowerNow(ctx context.Context, id uint, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return fmt.Errorf("%w: empty", ErrUnsupportedPowerAction)
	}
	if !isSupportedPowerAction(action) {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}

	t, err := s.store.Get(id)
	if err != nil {
		return err
	}
	c, ok := s.reg.CollectorFor(t.Type, ResolveProtocol(t))
	if !ok {
		return ErrUnsupported
	}
	pc, ok := c.(PowerController)
	if !ok {
		return fmt.Errorf("%w for %s/%s", ErrUnsupportedPowerAction, t.Type, ResolveProtocol(t))
	}
	creds, err := s.store.Credentials(t)
	if err != nil {
		return err
	}

	timeout := time.Duration(t.TimeoutSec) * time.Second
	if timeout < 5*time.Second {
		timeout = 15 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return pc.Power(runCtx, t, creds, action)
}

func isSupportedPowerAction(action string) bool {
	return action == PowerActionReboot || action == PowerActionShutdown
}
