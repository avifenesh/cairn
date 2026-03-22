package memory

// UserProfile wraps a MarkdownFile for the user's USER.md profile.
// The file describes the user's name, use case, and communication preferences.
type UserProfile struct {
	*MarkdownFile
}

// NewUserProfile creates a UserProfile bound to the given file path.
func NewUserProfile(filePath string) *UserProfile {
	return &UserProfile{MarkdownFile: NewMarkdownFile(filePath)}
}
