package importer

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Joplin's RAW export item-type codes (used both standalone and packed into
// a .jex, which is a plain tar archive despite the zip-like extension).
const (
	typeNote     = "1"
	typeFolder   = "2"
	typeResource = "4"
	typeTag      = "5"
	typeNoteTag  = "6"
)

// knownKVKeys is the fixed field set Joplin's exporter writes in a
// note/folder/tag/relation/resource item's trailing metadata block.
// Restricting the backward key:value scan to this allowlist (rather than
// accepting any "word: text" line) avoids misparsing body prose that happens
// to look like "Label: something".
var knownKVKeys = map[string]bool{
	"id": true, "parent_id": true, "created_time": true, "updated_time": true, "is_conflict": true,
	"latitude": true, "longitude": true, "altitude": true, "author": true, "source_url": true,
	"is_todo": true, "todo_due": true, "todo_completed": true, "source": true, "source_application": true,
	"application_data": true, "order": true, "user_created_time": true, "user_updated_time": true,
	"encryption_cipher_text": true, "encryption_applied": true, "markup_language": true, "is_shared": true,
	"share_id": true, "conflict_original_id": true, "master_key_id": true, "note_id": true, "tag_id": true,
	"mime": true, "filename": true, "size": true, "file_extension": true, "encryption_blob_encrypted": true,
	"icon": true, "type_": true,
}

var kvLineRE = regexp.MustCompile(`^([a-z_]+):(.*)$`)

type kvBlock struct {
	metadata           map[string]string
	contentBeforeBlock string
}

// splitKVBlock splits a Joplin RAW-export item's raw text into its leading
// content and trailing `key: value` metadata block. Scans backward from the
// last line, collecting recognized-key lines, and stops at the first line
// that either doesn't match `key: value` shape or whose key isn't in
// knownKVKeys — matching what a real export always has (every field
// present, including empty ones) so a genuine content/metadata boundary is
// never ambiguous.
func splitKVBlock(raw string) kvBlock {
	lines := strings.Split(raw, "\n")
	boundary := len(lines)

	for i := len(lines) - 1; i >= 0; i-- {
		m := kvLineRE.FindStringSubmatch(lines[i])
		if m == nil || !knownKVKeys[m[1]] {
			break
		}
		boundary = i
	}

	metadata := map[string]string{}
	for i := boundary; i < len(lines); i++ {
		if m := kvLineRE.FindStringSubmatch(lines[i]); m != nil {
			metadata[m[1]] = strings.TrimSpace(m[2])
		}
	}

	// The exporter puts one blank line between content and the metadata block.
	contentEnd := boundary
	if contentEnd > 0 && lines[contentEnd-1] == "" {
		contentEnd--
	}

	return kvBlock{metadata: metadata, contentBeforeBlock: strings.Join(lines[:contentEnd], "\n")}
}

// splitTitleAndBody: Joplin RAW items store title as a bare first line (no
// `#` heading marker, unlike the markdown importer's convention) — content
// is title, blank line, body. Folders/tags have title only (no body).
func splitTitleAndBody(content string) (title, body string) {
	idx := strings.IndexByte(content, '\n')
	if idx == -1 {
		return content, ""
	}
	return content[:idx], strings.TrimPrefix(content[idx+1:], "\n")
}

func toEpochMs(iso string) (int64, bool) {
	if iso == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return 0, false
	}
	return t.UnixMilli(), true
}

func stemOf(name string) string {
	if idx := strings.LastIndexByte(name, '.'); idx != -1 {
		return name[:idx]
	}
	return name
}

type namedBlob struct {
	name string
	data []byte
}

// ParseJexSource parses a .jex archive (a tar file containing a Joplin RAW
// export) entirely in memory into a ParsedImport, reconstructing notebook
// hierarchy, tags, and note-tag associations from the archive's own item
// types — the reference implementation this was ported from does none of
// that for JEX (only a literal inline `tags:` field, which real exports
// never set).
func ParseJexSource(jexPath string) (ParsedImport, error) {
	f, err := os.Open(jexPath)
	if err != nil {
		return ParsedImport{}, fmt.Errorf("not a valid JEX archive (expected a tar archive): %s — %s", jexPath, err.Error())
	}
	defer f.Close()

	var mdEntries []namedBlob
	var resourceEntries []namedBlob

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err != nil {
			// Go's archive/tar rejects non-tar content outright (unlike the
			// npm `tar` package this was ported from, which silently yields
			// zero entries for garbage input) — stop reading either way
			// (this also covers the normal io.EOF case) and let the
			// emptiness check below produce one consistent error regardless
			// of which library detected the problem, or how.
			break
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			break
		}
		// tar normalizes a `.`-cwd add with a `./` prefix on every entry path.
		entryPath := strings.TrimPrefix(hdr.Name, "./")
		switch {
		case strings.HasPrefix(entryPath, "resources/"):
			resourceEntries = append(resourceEntries, namedBlob{name: strings.TrimPrefix(entryPath, "resources/"), data: data})
		case strings.HasSuffix(entryPath, ".md"):
			mdEntries = append(mdEntries, namedBlob{name: entryPath, data: data})
		}
	}

	if len(mdEntries) == 0 && len(resourceEntries) == 0 {
		return ParsedImport{}, fmt.Errorf("empty or invalid JEX archive (no note/resource entries found, or not a valid tar archive): %s", jexPath)
	}

	var notebooks []ParsedNotebook
	var tags []ParsedTag
	type noteTagRelation struct{ noteID, tagID string }
	var noteTagRelations []noteTagRelation
	type resourceMetaEntry struct{ id, filename, mime string }
	var resourceMeta []resourceMetaEntry
	var notes []ParsedNote

	for _, entry := range mdEntries {
		kv := splitKVBlock(string(entry.data))
		typ := kv.metadata["type_"]
		if typ == "" {
			continue // not a recognizable Joplin export item — skip rather than guess.
		}

		title, body := splitTitleAndBody(kv.contentBeforeBlock)

		switch typ {
		case typeFolder:
			notebooks = append(notebooks, ParsedNotebook{Ref: kv.metadata["id"], Title: title, ParentRef: kv.metadata["parent_id"]})
		case typeTag:
			tags = append(tags, ParsedTag{Ref: kv.metadata["id"], Title: title})
		case typeNoteTag:
			if kv.metadata["note_id"] != "" && kv.metadata["tag_id"] != "" {
				noteTagRelations = append(noteTagRelations, noteTagRelation{noteID: kv.metadata["note_id"], tagID: kv.metadata["tag_id"]})
			}
		case typeResource:
			filename := kv.metadata["filename"]
			if filename == "" {
				filename = title
			}
			resourceMeta = append(resourceMeta, resourceMetaEntry{id: kv.metadata["id"], filename: filename, mime: kv.metadata["mime"]})
		case typeNote:
			notebookRef := kv.metadata["parent_id"]
			if notebookRef == "" {
				notebookRef = RootNotebookRef
			}
			note := ParsedNote{
				Title:         title,
				Body:          body,
				NotebookRef:   notebookRef,
				IsTodo:        kv.metadata["is_todo"] == "1",
				TodoCompleted: kv.metadata["todo_completed"] == "1",
				SourceID:      kv.metadata["id"],
			}
			if ms, ok := toEpochMs(kv.metadata["created_time"]); ok {
				note.CreatedTime = &ms
			} else if ms, ok := toEpochMs(kv.metadata["user_created_time"]); ok {
				note.CreatedTime = &ms
			}
			if ms, ok := toEpochMs(kv.metadata["updated_time"]); ok {
				note.UpdatedTime = &ms
			} else if ms, ok := toEpochMs(kv.metadata["user_updated_time"]); ok {
				note.UpdatedTime = &ms
			}
			notes = append(notes, note)
			// tagRefs filled in below once all note_tag relations are known.
		}
	}

	tagsByNoteID := map[string][]string{}
	for _, rel := range noteTagRelations {
		tagsByNoteID[rel.noteID] = append(tagsByNoteID[rel.noteID], rel.tagID)
	}
	for i := range notes {
		if notes[i].SourceID != "" {
			notes[i].TagRefs = tagsByNoteID[notes[i].SourceID]
		}
	}

	resourceByStem := map[string]namedBlob{}
	for _, r := range resourceEntries {
		resourceByStem[stemOf(r.name)] = r
	}

	var resources []ParsedResource
	for _, meta := range resourceMeta {
		blob, ok := resourceByStem[meta.id]
		if !ok {
			continue // metadata item with no matching binary — nothing to upload, left unresolved downstream.
		}
		filename := meta.filename
		if filename == "" {
			filename = blob.name
		}
		resources = append(resources, ParsedResource{ID: meta.id, Filename: filename, Mime: meta.mime, Data: blob.data})
	}

	return ParsedImport{Notebooks: notebooks, Tags: tags, Notes: notes, Resources: resources}, nil
}
