package google

import (
	"context"
	"strings"
	"testing"
)

type fakeDrive struct {
	files []File
	err   error
	gotQ  string
	gotN  int64
}

func (f *fakeDrive) search(_ context.Context, q string, n int64) ([]File, error) {
	f.gotQ, f.gotN = q, n
	return f.files, f.err
}

func TestDriveFind(t *testing.T) {
	fd := &fakeDrive{files: []File{
		{Name: "Bütçe 2026", Link: "https://drive.google.com/file/x"},
		{Name: "Notlar"},
	}}
	d := &Drive{api: fd}

	out, err := d.Execute(context.Background(), ActionFindFile, map[string]string{"query": "büt"})
	if err != nil {
		t.Fatal(err)
	}
	if fd.gotQ != "büt" {
		t.Errorf("query not forwarded: %q", fd.gotQ)
	}
	if !strings.Contains(out, "Bütçe 2026") || !strings.Contains(out, "https://drive.google.com/file/x") {
		t.Errorf("missing name/link in reply: %q", out)
	}
}

func TestDriveFindEmpty(t *testing.T) {
	d := &Drive{api: &fakeDrive{}}
	out, err := d.Execute(context.Background(), ActionFindFile, map[string]string{"query": "yok"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "eşleşen dosya yok") {
		t.Errorf("expected no-match reply, got %q", out)
	}
}

func TestDriveFindMissingQuery(t *testing.T) {
	d := &Drive{api: &fakeDrive{}}
	if _, err := d.Execute(context.Background(), ActionFindFile, map[string]string{}); err == nil {
		t.Fatal("expected error for empty query")
	}
}
