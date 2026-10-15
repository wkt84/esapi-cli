// Package diagnostic defines machine-readable diagnostics shared by commands.
package diagnostic

// Diagnostic codes and severity values are part of the CLI's versioned contract.
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Error preserves an underlying error for classification without parsing text.
type Error struct {
	Diagnostic
	Cause error
}

func New(code, message string, cause error) *Error {
	return &Error{Diagnostic: Diagnostic{Code: code, Severity: "error", Message: message}, Cause: cause}
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }
