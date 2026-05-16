package relay

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michmich112/congee/internal/config"
	"github.com/michmich112/congee/internal/storage/sqlite"
	"github.com/rs/zerolog"
)

func TestHookChainRunWrapsHookName(t *testing.T) {
	t.Parallel()
	var c HookChain
	c.Append("alpha", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		return errors.New("boom")
	})
	c.Append("beta", func(ctx context.Context, env HookEnv) error { return nil })
	err := c.Run(context.Background(), HookEnv{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "post_hook alpha") {
		t.Fatalf("want post_hook alpha in %q", err.Error())
	}
}

func TestHookChainPrependRunsBeforeAppend(t *testing.T) {
	t.Parallel()
	var c HookChain
	var order []string
	c.Append("append_first", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		order = append(order, "append_first")
		return nil
	})
	c.Prepend("prepend_late", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		order = append(order, "prepend_late")
		return nil
	})
	c.Append("append_second", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		order = append(order, "append_second")
		return nil
	})
	if err := c.Run(context.Background(), HookEnv{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepend_late", "append_first", "append_second"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("hook order = %v, want %v", order, want)
		}
	}
}

func TestHookChainRunStopsAtFirstErrorAndWrapsName(t *testing.T) {
	t.Parallel()
	var c HookChain
	var betaRan bool
	c.Append("first", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		return errors.New("stop")
	})
	c.Append("second", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		betaRan = true
		return nil
	})
	err := c.Run(context.Background(), HookEnv{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "post_hook first") {
		t.Fatalf("want wrapped post_hook first in %q", err.Error())
	}
	if betaRan {
		t.Fatal("second hook should not run after first error")
	}
}

func TestServerPostHooksDelegateToHookChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "hooks.db"), nil, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := config.DefaultConfig()
	srv, err := NewServer(cfg, st, zerolog.Nop(), nil)
	if err != nil {
		t.Fatal(err)
	}

	var order []string
	srv.AppendPostHook("app", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		order = append(order, "app")
		return nil
	})
	srv.PrependPostHook("pre", func(ctx context.Context, env HookEnv) error {
		_ = ctx
		_ = env
		order = append(order, "pre")
		return nil
	})

	if err := srv.hooks.Run(ctx, HookEnv{}); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "pre" || order[1] != "app" {
		t.Fatalf("got order %v, want [pre app]", order)
	}
}
