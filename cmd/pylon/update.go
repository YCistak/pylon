package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/YCistak/pylon/internal/i18n"
	"time"

	"github.com/YCistak/pylon/internal/selfupdate"
)

// cmdUpdate installs the newest release over this binary, or with --check only
// reports what is available.
//
// The daemon is deliberately not restarted here. It may be serving the GUI or
// mid-briefing, and a CLI invocation is the wrong place to decide that someone
// else's session ends now — so the new binary lands and the user restarts.
func cmdUpdate(args []string) error {
	checkOnly := false
	for _, a := range args {
		switch a {
		case "--check", "-check":
			checkOnly = true
		default:
			return fmt.Errorf("usage: pylon update [--check]")
		}
	}

	if by, packaged := selfupdate.Packaged(); packaged {
		// Not an error: the copy is up to date by a route that works better
		// than this one. Say which, so the next step is obvious.
		fmt.Println(i18n.T("update.packaged", by))
		fmt.Println(i18n.T("update.packaged_hint"))
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := selfupdate.NewClient()
	rel, newer, err := client.Check(ctx, version)
	switch {
	case errors.Is(err, selfupdate.ErrDevBuild):
		fmt.Println(i18n.T("update.dev_build"))
		return nil
	case errors.Is(err, selfupdate.ErrUpdatesDisabled):
		fmt.Println(i18n.T("update.unsigned"))
		return nil
	case err != nil:
		return err
	}

	if !newer {
		fmt.Println(i18n.T("update.current", version))
		return nil
	}
	fmt.Println(i18n.T("update.available", rel.Version, version))
	if checkOnly {
		return nil
	}

	fmt.Println(i18n.T("update.downloading"))
	if err := client.Apply(ctx, rel); err != nil {
		return err
	}
	fmt.Println(i18n.T("update.installed", rel.Version))
	return nil
}
