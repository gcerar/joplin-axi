package importer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var MDExtensions = []string{".md", ".markdown", ".mdown", ".mkd"}

var mdLinkRE = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

var mimeByExt = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".svg": "image/svg+xml", ".webp": "image/webp", ".pdf": "application/pdf", ".txt": "text/plain",
	".zip": "application/zip", ".mp3": "audio/mpeg", ".mp4": "video/mp4",
}

func guessMime(filename string) string {
	if m, ok := mimeByExt[strings.ToLower(filepath.Ext(filename))]; ok {
		return m
	}
	return "application/octet-stream"
}

func resolvePath(dir, target string) string {
	joined := filepath.Join(dir, target)
	if abs, err := filepath.Abs(joined); err == nil {
		return abs
	}
	return joined
}

// rewriteAssetLinks rewrites relative links pointing at a real local file
// that isn't another imported note into a `:/<synthetic-id>` token — the
// same shape JEX bodies already contain — registering the file in resources
// (deduped by resolved path, so a logo referenced from three notes is only
// read from disk once, in insertion order via resourceOrder). The synthetic
// ID is an MD5 hash of the resolved path: 32 hex characters, matching
// Joplin's own ID shape, which is what lets importer.go's existing `:/id`
// token resource-upload pass (built for JEX) handle these too with no
// changes of its own — a link to another imported .md note is deliberately
// left as-is here for that same pass to rewrite once note IDs exist. A
// relative link to a file that doesn't actually exist on disk is left
// untouched — not ours to guess about.
func rewriteAssetLinks(body, fileDir string, mdFileSet map[string]bool, resources map[string]*ParsedResource, resourceOrder *[]string) string {
	matches := mdLinkRE.FindAllStringSubmatchIndex(body, -1)
	if matches == nil {
		return body
	}

	var result strings.Builder
	lastIndex := 0
	for _, m := range matches {
		fullStart, fullEnd := m[0], m[1]
		text := body[m[2]:m[3]]
		target := body[m[4]:m[5]]

		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, ":/") {
			continue
		}

		resolved := resolvePath(fileDir, target)
		if mdFileSet[resolved] {
			continue // note-to-note link — importer.go's second pass handles this.
		}

		resource, ok := resources[resolved]
		if !ok {
			data, err := os.ReadFile(resolved)
			if err != nil {
				continue // doesn't exist / unreadable — leave the link exactly as written.
			}
			sum := md5.Sum([]byte(resolved))
			resource = &ParsedResource{ID: hex.EncodeToString(sum[:]), Filename: filepath.Base(resolved), Mime: guessMime(resolved), Data: data}
			resources[resolved] = resource
			*resourceOrder = append(*resourceOrder, resolved)
		}

		result.WriteString(body[lastIndex:fullStart])
		result.WriteString(fmt.Sprintf("[%s](:/%s)", text, resource.ID))
		lastIndex = fullEnd
	}
	result.WriteString(body[lastIndex:])
	return result.String()
}

func truthy(value string) bool {
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		return true
	}
	return false
}

// dateLayouts covers realistic frontmatter date formats. JS's Date.parse
// (what the TS version used) accepts a much broader set of formats — an
// accepted, narrow gap: no test in this project exercises anything beyond
// ISO 8601 date/datetime, which these layouts cover.
var dateLayouts = []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"}

func parseDateField(value string) *int64 {
	if value == "" {
		return nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			ms := t.UnixMilli()
			return &ms
		}
	}
	return nil
}

var headingRE = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*$`)

// extractTitle finds a title in body's first non-blank line, preferring a
// markdown heading (stripped from the returned body — Joplin displays the
// title separately, so leaving it in the body would show it twice), then a
// short first line, then fallback.
func extractTitle(body, fallback string) (title, newBody string) {
	lines := strings.Split(body, "\n")
	firstNonBlank := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			firstNonBlank = i
			break
		}
	}
	if firstNonBlank == -1 {
		return fallback, body
	}

	if m := headingRE.FindStringSubmatch(lines[firstNonBlank]); m != nil {
		rest := append(append([]string{}, lines[:firstNonBlank]...), lines[firstNonBlank+1:]...)
		joined := strings.TrimLeft(strings.Join(rest, "\n"), "\n")
		return m[1], joined
	}

	firstLine := strings.TrimSpace(lines[firstNonBlank])
	if runeLen := len([]rune(firstLine)); runeLen > 0 && runeLen <= 100 {
		return firstLine, body
	}

	return fallback, body
}

var underscoreDashRunRE = regexp.MustCompile(`[_-]+`)

func titleFromFilename(filePath string) string {
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	return strings.TrimSpace(underscoreDashRunRE.ReplaceAllString(base, " "))
}

func collectMarkdownFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var found []string
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := walk(full); err != nil {
					return err
				}
			} else if slices.Contains(MDExtensions, strings.ToLower(filepath.Ext(entry.Name()))) {
				found = append(found, full)
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return found, nil
}

// notebookRefForFile returns the notebook ref for a file: its directory
// path relative to the import root, with path separators normalized to `/`
// — or RootNotebookRef if the file sits directly at the root (or the source
// is a single file).
func notebookRefForFile(filePath, root string, isSingleFile bool) string {
	if isSingleFile {
		return RootNotebookRef
	}
	rel, err := filepath.Rel(root, filepath.Dir(filePath))
	if err != nil || rel == "." {
		return RootNotebookRef
	}
	return filepath.ToSlash(rel)
}

func ensureNotebookChain(notebooks map[string]*ParsedNotebook, order *[]string, ref string) {
	if ref == RootNotebookRef {
		return
	}
	if _, ok := notebooks[ref]; ok {
		return
	}
	segments := strings.Split(ref, "/")
	for i := range segments {
		segRef := strings.Join(segments[:i+1], "/")
		if _, ok := notebooks[segRef]; ok {
			continue
		}
		parentRef := ""
		if i > 0 {
			parentRef = strings.Join(segments[:i], "/")
		}
		notebooks[segRef] = &ParsedNotebook{Ref: segRef, Title: segments[i], ParentRef: parentRef}
		*order = append(*order, segRef)
	}
}

// ParseMarkdownSource parses a single markdown file or a directory of them
// into a ParsedImport. Directory structure becomes notebook structure
// (nested); frontmatter `notebook:` overrides that for an individual file.
// No Joplin API calls — see importer.go for the apply phase.
func ParseMarkdownSource(sourcePath string) (ParsedImport, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return ParsedImport{}, err
	}
	isSingleFile := !info.IsDir()

	if isSingleFile && !slices.Contains(MDExtensions, strings.ToLower(filepath.Ext(sourcePath))) {
		return ParsedImport{}, fmt.Errorf("not a markdown file (expected one of %s): %s", strings.Join(MDExtensions, ", "), sourcePath)
	}

	files, err := collectMarkdownFiles(sourcePath)
	if err != nil {
		return ParsedImport{}, err
	}

	mdFileSet := map[string]bool{}
	for _, f := range files {
		abs := f
		if a, err := filepath.Abs(f); err == nil {
			abs = a
		}
		mdFileSet[abs] = true
	}

	notebooks := map[string]*ParsedNotebook{}
	var notebookOrder []string
	tagSeen := map[string]bool{}
	var tagOrder []string
	resources := map[string]*ParsedResource{}
	var resourceOrder []string

	var notes []ParsedNote

	for _, filePath := range files {
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return ParsedImport{}, err
		}
		fm := ParseFrontmatter(string(raw), nil)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return ParsedImport{}, err
		}

		fallback := fm.Fields["title"]
		if fallback == "" {
			fallback = titleFromFilename(filePath)
		}
		if fallback == "" {
			fallback = "Untitled note"
		}
		extractedTitle, bodyWithoutTitle := extractTitle(fm.Body, fallback)
		body := rewriteAssetLinks(bodyWithoutTitle, filepath.Dir(filePath), mdFileSet, resources, &resourceOrder)
		resolvedTitle := fm.Fields["title"]
		if resolvedTitle == "" {
			resolvedTitle = extractedTitle
		}

		var notebookRef string
		if nbField := fm.Fields["notebook"]; nbField != "" {
			var segs []string
			for _, s := range strings.Split(nbField, "/") {
				if s = strings.TrimSpace(s); s != "" {
					segs = append(segs, s)
				}
			}
			notebookRef = strings.Join(segs, "/")
			if notebookRef == "" {
				notebookRef = RootNotebookRef
			}
		} else {
			notebookRef = notebookRefForFile(filePath, sourcePath, isSingleFile)
		}
		ensureNotebookChain(notebooks, &notebookOrder, notebookRef)

		tags := fm.Lists["tags"]
		for _, t := range tags {
			if !tagSeen[t] {
				tagSeen[t] = true
				tagOrder = append(tagOrder, t)
			}
		}

		createdTime := parseDateField(fm.Fields["created"])
		if createdTime == nil {
			createdTime = parseDateField(fm.Fields["date"])
		}
		if createdTime == nil {
			// Go's os.FileInfo doesn't portably expose file birth time — use
			// ModTime() as the fallback instead of a platform-specific shim.
			ms := fileInfo.ModTime().UnixMilli()
			createdTime = &ms
		}
		updatedTime := parseDateField(fm.Fields["updated"])
		if updatedTime == nil {
			updatedTime = parseDateField(fm.Fields["modified"])
		}
		if updatedTime == nil {
			ms := fileInfo.ModTime().UnixMilli()
			updatedTime = &ms
		}

		absPath := filePath
		if a, err := filepath.Abs(filePath); err == nil {
			absPath = a
		}

		notes = append(notes, ParsedNote{
			Title:          resolvedTitle,
			Body:           body,
			NotebookRef:    notebookRef,
			TagRefs:        tags,
			IsTodo:         truthy(fm.Fields["todo"]) || truthy(fm.Fields["is_todo"]),
			TodoCompleted:  truthy(fm.Fields["completed"]) || truthy(fm.Fields["todo_completed"]),
			CreatedTime:    createdTime,
			UpdatedTime:    updatedTime,
			SourceFilePath: absPath,
		})
	}

	result := ParsedImport{Notes: notes}
	for _, ref := range notebookOrder {
		result.Notebooks = append(result.Notebooks, *notebooks[ref])
	}
	for _, t := range tagOrder {
		result.Tags = append(result.Tags, ParsedTag{Ref: t, Title: t})
	}
	for _, p := range resourceOrder {
		result.Resources = append(result.Resources, *resources[p])
	}
	return result, nil
}
