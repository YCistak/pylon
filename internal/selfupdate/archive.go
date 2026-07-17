package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
)

// maxFileSize caps what we will hold from an archive. The binaries are ~30 MB;
// this stops a malformed or hostile archive from being decompressed into
// memory without bound. The signature check runs first, so this is a belt on
// top of braces — but a verified archive is still an archive we then parse.
const maxFileSize = 256 << 20

// extract pulls the named files out of a release archive, keyed by base name.
// Only the entries asked for are read, so the rest of the archive — config,
// docs, a macOS .app tree — costs nothing.
func extract(data []byte, want map[string]bool) (map[string][]byte, error) {
	if len(data) > 1 && data[0] == 'P' && data[1] == 'K' {
		return extractZip(data, want)
	}
	return extractTarGz(data, want)
}

func extractTarGz(data []byte, want map[string]bool) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer gz.Close()

	out := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Base(h.Name)
		if !want[name] {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(tr, maxFileSize))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		out[name] = b
	}
	return out, nil
}

func extractZip(data []byte, want map[string]bool) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	out := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Compare on the base name: the archive nests everything under a
		// versioned directory, and on macOS the GUI lives inside a .app bundle.
		name := path.Base(strings.ReplaceAll(f.Name, `\`, "/"))
		if !want[name] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxFileSize))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		out[name] = b
	}
	return out, nil
}
