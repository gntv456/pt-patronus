package chain

import (
	"context"
	"reflect"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
)

type cMod struct {
	name string
	prio int
}

func (m cMod) Name() string  { return m.name }
func (m cMod) Priority() int { return m.prio }

func TestRunModuleThreeSemantics(t *testing.T) {
	mm := module.New()
	mm.Register(cMod{"site", 1}, cMod{"site2", 2})
	b := NewBase(mm, event.New())

	// Continue: 收集全部
	got := RunModule(context.Background(), &b, func(mod module.Module) (string, module.Control) {
		return mod.Name(), module.Continue
	})
	if !reflect.DeepEqual(got, []string{"site", "site2"}) {
		t.Errorf("collect = %v", got)
	}

	// Stop: 第一个命中即停
	got = RunModule(context.Background(), &b, func(mod module.Module) (string, module.Control) {
		return mod.Name(), module.Stop
	})
	if !reflect.DeepEqual(got, []string{"site"}) {
		t.Errorf("stop = %v", got)
	}
}

func TestEmitBroadcast(t *testing.T) {
	bus := event.New()
	b := NewBase(module.New(), bus)
	hits := 0
	bus.Subscribe("test", 0, func(ctx context.Context, e event.Event) { hits++ })
	b.Emit(context.Background(), "test", nil)
	if hits != 1 {
		t.Fatalf("emit hits = %d", hits)
	}
}
