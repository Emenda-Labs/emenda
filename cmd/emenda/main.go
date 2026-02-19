package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/mod/semver"

	"github.com/emenda-labs/emenda/core/cli"
	golangdriver "github.com/emenda-labs/emenda/drivers/golang"
	"github.com/emenda-labs/emenda/pkg/gomod"
)

const version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	goDriver := golangdriver.NewDriver()

	runUpgradeGo := func(ctx context.Context, opts cli.UpgradeGoOptions) error {
		currentVersion, err := gomod.FindModuleVersion(opts.Repo, opts.Module)
		if err != nil {
			return err
		}

		if currentVersion == opts.To {
			return fmt.Errorf("module %s is already at %s", opts.Module, opts.To)
		}

		if semver.IsValid(currentVersion) && semver.IsValid(opts.To) {
			if semver.Compare(opts.To, currentVersion) < 0 {
				fmt.Fprintf(os.Stderr, "warning: target version %s is older than current version %s\n", opts.To, currentVersion)
			}
		}

		fmt.Fprintf(os.Stderr, "Downloading %s@%s...\n", opts.Module, currentVersion)
		oldPath, oldCleanup, err := goDriver.FetchSource(ctx, opts.Module, currentVersion)
		if err != nil {
			return fmt.Errorf("fetching old version: %w", err)
		}
		defer oldCleanup()

		fmt.Fprintf(os.Stderr, "Downloading %s@%s...\n", opts.Module, opts.To)
		newPath, newCleanup, err := goDriver.FetchSource(ctx, opts.Module, opts.To)
		if err != nil {
			return fmt.Errorf("fetching new version: %w", err)
		}
		defer newCleanup()

		fmt.Printf("Module:          %s\n", opts.Module)
		fmt.Printf("Current version: %s\n", currentVersion)
		fmt.Printf("Target version:  %s\n", opts.To)
		fmt.Printf("Old source:      %s\n", oldPath)
		fmt.Printf("New source:      %s\n", newPath)
		fmt.Println()

		if opts.DryRun {
			fmt.Println("[dry-run] No changes applied.")
		} else {
			fmt.Println("[no-op] Change application not yet implemented.")
		}

		return nil
	}

	root := cli.NewRootCmd(version)
	upgradeCmd := cli.NewUpgradeCmd()
	upgradeCmd.AddCommand(cli.NewUpgradeGoCmd(runUpgradeGo))
	root.AddCommand(upgradeCmd)

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
