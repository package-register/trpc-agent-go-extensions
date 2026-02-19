package prompt

import "strings"

// FormatLayerMarker returns the XML tag used to identify Layer 1+2 content
// in system messages. Used by Compressor to identify protected content.
func FormatLayerMarker() string {
	return "<system_core_prompt>"
}

// IsProtectedSystemMessage checks if a system message contains Layer 1+2 content
// that should never be compressed.
func IsProtectedSystemMessage(content string) bool {
	return strings.Contains(content, "<system_core_prompt>") ||
		strings.Contains(content, "<pkg_inject_prompt>")
}
