// Package importer implements joplin-axi's two-phase import pipeline: a
// source-specific parse phase (jexsource.go / markdownsource.go) produces a
// ParsedImport with zero Joplin API calls, then Run (importer.go) applies
// it. Keeping parsing pure and synchronous-with-the-filesystem-only is what
// makes --dry-run (parse, then stop) and unit-testing the parsers (no
// stubbed client needed) both cheap.
package importer

// RootNotebookRef is the sentinel NotebookRef meaning "the --notebook target
// itself" rather than a notebook that needs creating: for markdown import,
// notes/directories at the source root; for JEX import, items with no
// parent_id (Joplin's own concept of a top-level item). Shared by both
// sources so importer.go has one rule regardless of which produced the
// ParsedImport.
const RootNotebookRef = "."

// ParsedNotebook. Ref is a stable key used elsewhere in the same
// ParsedImport to reference this notebook — a literal Joplin ID for JEX
// (from its type_=2 items), or a "/"-joined path for markdown (derived from
// directory structure). Never sent to Joplin as-is. ParentRef is "" for a
// top-level notebook — no real ref is ever empty, so this is an unambiguous
// sentinel (mirrors the TS parentRef?: string).
type ParsedNotebook struct {
	Ref       string
	Title     string
	ParentRef string
}

// ParsedTag. Ref is a literal Joplin ID (JEX) or the title itself (markdown,
// which has no separate tag IDs — frontmatter/hashtags are just strings).
type ParsedTag struct {
	Ref   string
	Title string
}

// ParsedNote. CreatedTime/UpdatedTime are nil when genuinely unset (mirrors
// TS's `?: number` — distinct from a zero epoch, which the apply phase's
// timestamp pass must still send to Joplin). SourceID (JEX only) and
// SourceFilePath (markdown only) are "" when not applicable to that source —
// again unambiguous, since neither a real Joplin ID nor a real absolute path
// is ever empty.
type ParsedNote struct {
	Title          string
	Body           string
	NotebookRef    string
	TagRefs        []string
	IsTodo         bool
	TodoCompleted  bool
	CreatedTime    *int64
	UpdatedTime    *int64
	SourceID       string
	SourceFilePath string
}

// ParsedResource. ID is the original Joplin resource ID (JEX only) — what
// `:/id` tokens reference. Mime is "" when unknown.
type ParsedResource struct {
	ID       string
	Filename string
	Mime     string
	Data     []byte
}

type ParsedImport struct {
	Notebooks []ParsedNotebook
	Tags      []ParsedTag
	Notes     []ParsedNote
	Resources []ParsedResource
}

// TitledReason records a skipped note — title plus a human-readable reason.
type TitledReason struct {
	Title  string
	Reason string
}

// TitledError records a failed note or link-rewrite — title plus the error
// message. A separate type from TitledReason (rather than reusing one shape
// for both) mirrors the TS source's two distinctly-named {title, reason} vs
// {title, error} shapes.
type TitledError struct {
	Title string
	Error string
}

type ImportReport struct {
	NotebooksCreated int
	TagsCreated      int
	NotesCreated     int
	NotesSkipped     []TitledReason
	NotesFailed      []TitledError
	// ResourcesUploaded and UnresolvedLinks are counts, not lists — nothing
	// downstream needs to know *which* link/resource, only how many.
	ResourcesUploaded int
	UnresolvedLinks   int
	// LinkRewriteFailed: the note was created successfully (counted in
	// NotesCreated) but rewriting its links/resource tokens afterward
	// failed — its body may still contain stale `:/oldId` references.
	// Distinct from NotesFailed, where the note was never created at all.
	LinkRewriteFailed []TitledError
}

// OnDuplicate is a closed set (skip/rename) modeled as typed string
// constants — Go has no union types. The one entry point that should ever
// construct one is the import command's own flag validation.
type OnDuplicate string

const (
	OnDuplicateSkip   OnDuplicate = "skip"
	OnDuplicateRename OnDuplicate = "rename"
)
