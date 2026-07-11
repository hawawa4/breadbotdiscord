package bot

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// TestAttachmentFilenameDisambiguates is the regression guard for the
// multi-image clobber bug: several attachments named "image.png" must produce
// distinct local filenames so none overwrites another on disk.
func TestAttachmentFilenameDisambiguates(t *testing.T) {
	a1 := &discordgo.MessageAttachment{ID: "111", Filename: "image.png"}
	a2 := &discordgo.MessageAttachment{ID: "222", Filename: "image.png"}

	n1 := attachmentFilename(a1)
	n2 := attachmentFilename(a2)
	if n1 == n2 {
		t.Fatalf("two 'image.png' attachments collided: both -> %q", n1)
	}
	if !strings.Contains(n1, "111") || !strings.Contains(n2, "222") {
		t.Errorf("filenames should carry the attachment id: %q, %q", n1, n2)
	}
	if !strings.HasSuffix(n1, "image.png") {
		t.Errorf("original name should be preserved, got %q", n1)
	}
}

func TestAttachmentFilenameNoID(t *testing.T) {
	// Missing id (shouldn't happen, but be defensive): fall back to sanitized name.
	a := &discordgo.MessageAttachment{Filename: "loaf.png"}
	if got := attachmentFilename(a); got != "loaf.png" {
		t.Errorf("no-id filename = %q, want loaf.png", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"image.png":        "image.png",
		"a/b/c.png":        "c.png",  // directory parts dropped
		"../../etc/passwd": "passwd", // traversal neutralized
		"":                 "file",   // empty fallback
		".":                "file",   // dot fallback
		"..":               "file",   // dotdot fallback
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
