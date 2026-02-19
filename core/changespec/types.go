package changespec

// ChangeKind represents the type of breaking API change.
type ChangeKind string

const (
	ChangeKindRenamed          ChangeKind = "renamed"
	ChangeKindSignatureChanged ChangeKind = "signature_changed"
	ChangeKindRemoved          ChangeKind = "removed"
	ChangeKindTypeChanged      ChangeKind = "type_changed"
	ChangeKindPackageMoved     ChangeKind = "package_moved"
)

// Change represents a single breaking API change between two versions.
type Change struct {
	Kind         ChangeKind `json:"kind"`
	Symbol       string     `json:"symbol"`
	Package      string     `json:"package"`
	OldSignature string     `json:"old_signature,omitempty"`
	NewSignature string     `json:"new_signature,omitempty"`
	NewName      string     `json:"new_name,omitempty"`
	NewPackage   string     `json:"new_package,omitempty"`
}

// ChangeSpec is the full set of breaking changes between two module versions.
type ChangeSpec struct {
	Module     string   `json:"module"`
	OldVersion string   `json:"old_version"`
	NewVersion string   `json:"new_version"`
	Changes    []Change `json:"changes"`
}

// ApplyResult reports which changes were successfully applied and which failed.
// Failed changes may be sent to the AI agent path for resolution.
type ApplyResult struct {
	Applied []Change `json:"applied"`
	Failed  []Change `json:"failed"`
}
