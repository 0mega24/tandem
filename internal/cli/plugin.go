package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	ucli "github.com/urfave/cli/v3"

	"github.com/0mega24/tandem/internal/config"
	"github.com/0mega24/tandem/internal/gen"
	"github.com/0mega24/tandem/internal/gen/external"
)

// loadGenerators registers plugins before checking enabled IDs and generator tables.
func loadGenerators(cmd *ucli.Command) (config.Loaded, config.Env, error) {
	env, err := config.EnvFromOS()
	if err != nil {
		return config.Loaded{}, env, err
	}
	loaded, err := config.Load(env, cmd.String("config"))
	if err != nil {
		return loaded, env, err
	}
	plugins, err := external.Discover(loaded.Config.Generate.PluginDirs, os.Getenv("PATH"))
	if err != nil {
		return loaded, env, err
	}
	for _, p := range plugins {
		if _, ok := gen.ByID(p.Meta().ID); ok {
			return loaded, env, fmt.Errorf("config: plugin ID %q conflicts with a built-in generator", p.Meta().ID)
		}
	}
	for _, p := range plugins {
		p.Stderr = cmd.Root().ErrWriter
		gen.Register(p)
	}
	if err := gen.CheckConfig(loaded.Config.Generate.Enabled, loaded.Config.Generate.Tables); err != nil {
		return loaded, env, fmt.Errorf("config: %w", err)
	}
	return loaded, env, nil
}

func pluginCommand() *ucli.Command {
	return &ucli.Command{
		Name:     "plugin",
		Usage:    "Inspect external generator plugins",
		Commands: []*ucli.Command{{Name: "check", Usage: "Validate meta and a plan for a test space", ArgsUsage: "<id>", Action: checkPlugin}},
	}
}

func checkPlugin(_ context.Context, cmd *ucli.Command) (err error) {
	if cmd.Args().Len() != 1 {
		return errors.New("usage: tdm plugin check <id>")
	}
	loaded, _, err := loadGenerators(cmd)
	if err != nil {
		return err
	}
	id := cmd.Args().First()
	g, ok := gen.ByID(id)
	if !ok {
		return fmt.Errorf("unknown plugin %q", id)
	}
	p, ok := g.(external.Plugin)
	if !ok {
		return fmt.Errorf("%s is a built-in generator, not an external plugin", id)
	}
	if _, err := fmt.Fprintf(cmd.Root().Writer, "%s: meta passed (protocol 1)\n", id); err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "tdm-plugin-check-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(root)) }()
	member := filepath.Join(root, "member")
	if err := os.Mkdir(member, 0o700); err != nil {
		return err
	}
	_, err = p.Plan(gen.Space{Name: "plugin-check", Root: root, Branch: "plugin-check", Members: []gen.Member{{Name: "member", WorktreePath: member}}, Config: loaded.Config.Generate.Table(id)}, nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.Root().Writer, "%s: plan passed (schema and paths)\n", id)
	return err
}
