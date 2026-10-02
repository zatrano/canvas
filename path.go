package canvas

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const maxTemplateRelLen = 512

// secureRelPath maps a Canvas view name to a rooted-relative path for loading.
// Rejects empty names, NUL, absolute paths, volume paths, and any ".." segment.
//
// OS rules:
//   - Windows: '\' is a separator (via filepath); IsLocal rejects volumes / ..
//   - Unix: '\' is a normal filename character, not a separator — a name like
//     `foo\bar` is a single local element (io/fs.ValidPath forbids '\', so those
//     names are read via os.ReadFile after a local-element check).
//
// go 1.22: symbolic links that point outside the root are NOT blocked by DirFS;
// see SECURITY.md. Go 1.24+ may adopt os.Root for that case.
func (e *Engine) secureRelPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("canvas: empty template name")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("canvas: template name contains NUL")
	}
	if len(name) > maxTemplateRelLen {
		return "", fmt.Errorf("canvas: template name too long")
	}

	rel := name
	hasSlash := strings.Contains(rel, "/")
	hasBack := strings.Contains(rel, `\`)

	switch {
	case runtime.GOOS == "windows":
		if hasSlash || hasBack {
			rel = filepath.ToSlash(rel)
		} else {
			rel = strings.ReplaceAll(rel, ".", "/")
		}
	case hasSlash:
		// Unix path-like: keep slashes; do not rewrite dots.
	case hasBack:
		// Unix: backslash is part of the filename — keep as one element.
	default:
		rel = strings.ReplaceAll(rel, ".", "/")
	}

	if e.extension != "" && !strings.HasSuffix(rel, e.extension) {
		rel += e.extension
	}

	if runtime.GOOS != "windows" && !strings.Contains(rel, "/") && strings.Contains(rel, `\`) {
		if rel == ".." || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("canvas: template path escapes root or is invalid: %q", name)
		}
		if len(rel) > maxTemplateRelLen {
			return "", fmt.Errorf("canvas: template name too long")
		}
		return rel, nil
	}

	rel = path.Clean(rel)
	if rel == "." || rel == "" {
		return "", fmt.Errorf("canvas: invalid template path %q", name)
	}
	if !fs.ValidPath(rel) {
		return "", fmt.Errorf("canvas: template path escapes root or is invalid: %q", name)
	}
	if !filepath.IsLocal(filepath.FromSlash(rel)) {
		return "", fmt.Errorf("canvas: template path escapes root or is invalid: %q", name)
	}
	if len(rel) > maxTemplateRelLen {
		return "", fmt.Errorf("canvas: template name too long")
	}
	return rel, nil
}

func (e *Engine) pathFor(name string) string {
	rel, err := e.secureRelPath(name)
	if err != nil {
		return filepath.Join(e.directory, name)
	}
	return filepath.Join(e.directory, filepath.FromSlash(rel))
}

func (e *Engine) readTemplate(name string) ([]byte, error) {
	rel, err := e.secureRelPath(name)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && strings.Contains(rel, `\`) {
		full := filepath.Clean(filepath.Join(e.directory, rel))
		root := filepath.Clean(e.directory)
		if full != root && !isUnderDir(full, root) {
			return nil, fmt.Errorf("canvas: template path escapes root: %q", name)
		}
		return os.ReadFile(full)
	}
	return fs.ReadFile(os.DirFS(e.directory), rel)
}

func (e *Engine) templateExists(name string) bool {
	rel, err := e.secureRelPath(name)
	if err != nil {
		return false
	}
	if runtime.GOOS != "windows" && strings.Contains(rel, `\`) {
		fi, err := os.Stat(filepath.Join(e.directory, rel))
		return err == nil && !fi.IsDir()
	}
	f, err := fs.Stat(os.DirFS(e.directory), rel)
	return err == nil && !f.IsDir()
}

func isUnderDir(full, root string) bool {
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}
