package bot

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// MessagePreview is a minimal, discordgo-free snapshot of a Discord message,
// enough for the web UI to render a faked message card with its images inline.
// It deliberately carries only what a preview needs (author, text, timestamp,
// image URLs) rather than the full message object.
type MessagePreview struct {
	MessageID   string   // the message's snowflake id
	ChannelID   string   // channel the message lives in
	AuthorName  string   // display name (member nick if available, else username)
	AuthorAvatar string   // avatar URL, or "" if none
	Content     string   // the message text
	TimestampMs int64    // creation time, unix milliseconds
	ImageURLs   []string // freshly-signed CDN URLs for image attachments + embeds
}

// FetchMessagePreview fetches a single message over the REST API and reduces it
// to a MessagePreview. The image URLs come straight from Discord's CDN with the
// fresh signature params (ex/is/hm) attached by this REST fetch, so the browser
// can load them directly for as long as the signature is valid — we do not
// proxy or persist them. Returns an error if the message can't be fetched.
func (b *Bot) FetchMessagePreview(channelID, messageID string) (MessagePreview, error) {
	m, err := b.session.ChannelMessage(channelID, messageID)
	if err != nil {
		return MessagePreview{}, err
	}

	p := MessagePreview{
		MessageID: m.ID,
		ChannelID: channelID,
		Content:   m.Content,
	}
	if m.Author != nil {
		// Prefer a global display name, falling back to the username. (Guild nick
		// isn't populated on a REST-fetched message; username is always present.)
		p.AuthorName = m.Author.GlobalName
		if p.AuthorName == "" {
			p.AuthorName = m.Author.Username
		}
		p.AuthorAvatar = m.Author.AvatarURL("128")
	}
	if !m.Timestamp.IsZero() {
		p.TimestampMs = m.Timestamp.UnixMilli()
	}

	// Image attachments: use the content type when present, else sniff the
	// filename extension (some older attachments have no content type).
	for _, a := range m.Attachments {
		if isImageAttachment(a) {
			p.ImageURLs = append(p.ImageURLs, a.URL)
		}
	}
	// Embedded images/thumbnails (e.g. a linked image) also count as previewable.
	for _, e := range m.Embeds {
		if e.Image != nil && e.Image.URL != "" {
			p.ImageURLs = append(p.ImageURLs, e.Image.URL)
		} else if e.Thumbnail != nil && e.Thumbnail.URL != "" {
			p.ImageURLs = append(p.ImageURLs, e.Thumbnail.URL)
		}
	}

	return p, nil
}

// isImageAttachment reports whether an attachment is an image, by content type
// when available or filename extension otherwise.
func isImageAttachment(a *discordgo.MessageAttachment) bool {
	if a.ContentType != "" {
		return strings.HasPrefix(a.ContentType, "image/")
	}
	name := strings.ToLower(a.Filename)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
