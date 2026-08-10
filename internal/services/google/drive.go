package google

import (
	"context"
	"errors"
	"fmt"
	"strings"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/YCistak/pylon/internal/i18n"
	"github.com/YCistak/pylon/internal/intent"
)

// Drive actions.
const (
	ActionFindFile    intent.Action = "drive.find"
	ActionRecentFiles intent.Action = "drive.recent"
)

// recentFilesLimit caps how many files drive.recent lists (widget/voice reply
// should stay short).
const recentFilesLimit = 5

// File is the minimal shape Pylon needs from a Drive file (decoupled from the
// API for testing).
type File struct {
	Name string
	Link string // webViewLink — opens the file in the browser
}

// driveAPI is the slice of the Drive API the service uses; a fake implements it
// in tests.
type driveAPI interface {
	search(ctx context.Context, query string, limit int64) ([]File, error)
	recent(ctx context.Context, limit int64) ([]File, error)
}

// Drive is the Google Drive Service. It shares the Google OAuth client/token
// with Calendar (same Config, same httpClient) — Drive just needs the extra
// read-only metadata scope granted at `pylon auth google` time.
type Drive struct {
	cfg Config
	api driveAPI // injected in tests; otherwise built lazily from the token
}

// NewDrive builds the service from config. It does not touch the network or
// token until first use.
func NewDrive(cfg Config) *Drive { return &Drive{cfg: cfg} }

func (d *Drive) Name() string { return "drive" }

func (d *Drive) Actions() []intent.ActionSpec {
	return []intent.ActionSpec{
		{
			Name: ActionFindFile,
			Args: []string{"query"},
			Desc: `"drive.find": find a file in the user's Google Drive by name. Put the search text in "query". Use for "Drive'da bütçe dosyasını bul", "sunum dosyam nerede".`,
		},
		{
			Name: ActionRecentFiles,
			Desc: `"drive.recent": list the user's most recently modified Google Drive files. No args. Use for "Drive'da son dosyalarım ne", "en son neye dokunmuştum".`,
		},
	}
}

func (d *Drive) Execute(ctx context.Context, action intent.Action, args map[string]string) (string, error) {
	switch action {
	case ActionFindFile:
		return d.find(ctx, args)
	case ActionRecentFiles:
		return d.recent(ctx)
	default:
		return "", fmt.Errorf("drive: unknown action %q", action)
	}
}

func (d *Drive) find(ctx context.Context, args map[string]string) (string, error) {
	query := strings.TrimSpace(args["query"])
	if query == "" {
		return "", errors.New("drive: a file name to search for is required")
	}
	api, err := d.client(ctx)
	if err != nil {
		return "", err
	}
	files, err := api.search(ctx, query, recentFilesLimit)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return i18n.T("drive.no_match", query), nil
	}
	return fmt.Sprintf("Drive'da %d dosya bulundu: %s", len(files), formatFileList(files)), nil
}

func (d *Drive) recent(ctx context.Context) (string, error) {
	api, err := d.client(ctx)
	if err != nil {
		return "", err
	}
	files, err := api.recent(ctx, recentFilesLimit)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "Drive'da dosya yok.", nil
	}
	return "Son dosyalar: " + formatFileList(files), nil
}

// formatFileList renders files as "Name — link" (or just Name), joined for a
// speakable/glanceable reply. Shared by find and recent.
func formatFileList(files []File) string {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		if f.Link != "" {
			parts = append(parts, fmt.Sprintf("%s — %s", f.Name, f.Link))
		} else {
			parts = append(parts, f.Name)
		}
	}
	return strings.Join(parts, "; ")
}

// client lazily builds the real Drive API from the saved OAuth token, unless one
// was injected (tests).
func (d *Drive) client(ctx context.Context) (driveAPI, error) {
	if d.api != nil {
		return d.api, nil
	}
	hc, err := httpClient(ctx, d.cfg)
	if err != nil {
		return nil, err
	}
	svc, err := drive.NewService(ctx, option.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("drive service: %w", err)
	}
	return &realDrive{svc: svc}, nil
}

// realDrive adapts the google drive API to driveAPI.
type realDrive struct{ svc *drive.Service }

func (r *realDrive) search(ctx context.Context, query string, limit int64) ([]File, error) {
	// Drive query language: match by name, skip trashed. Escape single quotes so
	// a name with an apostrophe can't break the query string.
	safe := strings.ReplaceAll(query, `'`, `\'`)
	q := fmt.Sprintf("name contains '%s' and trashed = false", safe)
	return r.list(ctx, q, limit)
}

func (r *realDrive) recent(ctx context.Context, limit int64) ([]File, error) {
	return r.list(ctx, "trashed = false", limit)
}

func (r *realDrive) list(ctx context.Context, q string, limit int64) ([]File, error) {
	res, err := r.svc.Files.List().
		Context(ctx).
		Q(q).
		PageSize(limit).
		OrderBy("modifiedTime desc").
		Fields("files(name, webViewLink)").
		Do()
	if err != nil {
		return nil, err
	}
	out := make([]File, 0, len(res.Files))
	for _, f := range res.Files {
		out = append(out, File{Name: f.Name, Link: f.WebViewLink})
	}
	return out, nil
}
