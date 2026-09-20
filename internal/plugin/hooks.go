package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	sdk "github.com/michmich112/congee/sdk/plugin"
)

const (
	hookInstallTimeout = 15 * time.Minute
	hookLaunchTimeout  = 15 * time.Minute
)

func pluginProcessEnv(id, pkgDir, dataDir, settingsJSON string) []string {
	return append(os.Environ(),
		sdk.EnvPluginDataDir+"="+dataDir,
		sdk.EnvPluginID+"="+id,
		sdk.EnvPluginPackageDir+"="+pkgDir,
		sdk.EnvPluginSettings+"="+settingsJSON,
	)
}

// runManifestHook execs the plugin binary with extra args (install/launch). A missing
// hook is a no-op. Failure is returned; the caller decides whether to continue.
func runManifestHook(ctx context.Context, man *Manifest, pkgDir, dataDir, id, settingsJSON string, args []string, timeout time.Duration) error {
	if man == nil || len(args) == 0 {
		return nil
	}
	if timeout <= 0 {
		timeout = hookInstallTimeout
	}
	bin, err := man.execPath(pkgDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hctx, bin, args...)
	cmd.Dir = pkgDir
	cmd.Env = pluginProcessEnv(id, pkgDir, dataDir, settingsJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := stderr.String() + stdout.String()
		if len(out) > 800 {
			out = out[:800] + "…"
		}
		if out == "" {
			return fmt.Errorf("plugin hook %v: %w", args, err)
		}
		return fmt.Errorf("plugin hook %v: %w: %s", args, err, out)
	}
	return nil
}
