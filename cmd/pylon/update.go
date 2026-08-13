package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/YCistak/pylon/internal/daemon"
	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/ipc"
	"github.com/YCistak/pylon/internal/selfupdate"
)

// updateTimeout covers the whole round trip: the release check, the checksums,
// their signature, and a ~15 MB archive over whatever connection is there.
const updateTimeout = 2 * time.Minute

// updateStatus is the answer to "is there an update, and can this copy take it".
// It exists so the CLI and the GUI cannot drift apart in what they tell people:
// the wording, and the decision about whether installing is even offered, are
// made once here rather than twice.
type updateStatus struct {
	Message   string             // already translated, ready to show
	Available bool               // newer, and installable by this route
	Release   selfupdate.Release // only meaningful when Available
}

// checkForUpdate reports what an update would do, without doing any of it.
//
// The three "not applicable" outcomes are deliberately not errors. A copy
// installed by pacman, a build from a checkout, and a build with no signing key
// are all working installations doing the right thing; presenting them as
// failures would tell the user to fix something that is not broken.
func checkForUpdate(ctx context.Context) (updateStatus, error) {
	if by, packaged := selfupdate.Packaged(); packaged {
		return updateStatus{
			Message: i18n.T("update.packaged", by) + "\n" + i18n.T("update.packaged_hint"),
		}, nil
	}

	rel, newer, err := selfupdate.NewClient().Check(ctx, version)
	switch {
	case errors.Is(err, selfupdate.ErrDevBuild):
		return updateStatus{Message: i18n.T("update.dev_build")}, nil
	case errors.Is(err, selfupdate.ErrUpdatesDisabled):
		return updateStatus{Message: i18n.T("update.unsigned")}, nil
	case errors.Is(err, selfupdate.ErrNoRelease):
		return updateStatus{Message: i18n.T("update.none_published")}, nil
	case err != nil:
		return updateStatus{}, err
	}

	if !newer {
		return updateStatus{Message: i18n.T("update.current", version)}, nil
	}
	return updateStatus{
		Message:   i18n.T("update.available", rel.Version, version),
		Available: true,
		Release:   rel,
	}, nil
}

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

	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	st, err := checkForUpdate(ctx)
	if err != nil {
		return err
	}
	fmt.Println(st.Message)
	if !st.Available || checkOnly {
		return nil
	}

	fmt.Println(i18n.T("update.downloading"))
	if err := selfupdate.NewClient().Apply(ctx, st.Release); err != nil {
		return err
	}
	fmt.Println(i18n.T("update.installed", st.Release.Version))
	return nil
}

// registerUpdate exposes the same two steps over IPC, which is what the GUI's
// Hakkında tab calls. They are separate commands rather than one because the
// user has to be told what is about to be installed before it is: an update is
// the one action Pylon takes that replaces itself on disk.
//
// It runs inside the daemon rather than in the GUI for a reason the GUI cannot
// work around — a process cannot overwrite the binary it is running from on
// Windows, and cannot restart into a new one without losing its state. The
// daemon is a separate process, so it can replace both.
//
// "version" is here too, because the screen that offers the update is also the
// screen that has to say what is installed now.
func registerUpdate(d *daemon.Daemon) {
	d.Handle("version", func(ipc.Request) ipc.Response {
		return ipc.Response{OK: true, Text: version}
	})

	d.Handle("update", func(req ipc.Request) ipc.Response {
		apply := len(req.Args) > 0 && req.Args[0] == "apply"

		ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
		defer cancel()

		st, err := checkForUpdate(ctx)
		if err != nil {
			return ipc.Response{OK: false, Error: err.Error()}
		}
		if !apply || !st.Available {
			// "<available>\t<version>\t<message>", the same shape "lang state"
			// uses. The message alone is not enough: the GUI has to decide
			// whether to offer an Install button, and deciding that by matching
			// on translated prose would break in six languages at once.
			return ipc.Response{OK: true, Text: fmt.Sprintf("%t\t%s\t%s",
				st.Available, st.Release.Version, st.Message)}
		}

		if err := selfupdate.NewClient().Apply(ctx, st.Release); err != nil {
			return ipc.Response{OK: false, Error: err.Error()}
		}
		return ipc.Response{OK: true, Text: i18n.T("update.installed", st.Release.Version)}
	})
}
