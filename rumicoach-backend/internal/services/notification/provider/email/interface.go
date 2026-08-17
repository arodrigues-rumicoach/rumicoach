package email

type Provider interface {
	// SendEmail sends a multipart email. htmlBody is the rendered HTML version;
	// textBody is the plain-text alternative shown by text-only clients.
	SendEmail(toEmail, subject, htmlBody, textBody string) error
	// SendEmailWithSender allows overriding the default fromEmail and fromName.
	SendEmailWithSender(fromName, fromEmail, toEmail, subject, htmlBody, textBody string) error
	// SendEmailWithAttachments sends the same multipart email with files attached.
	//
	// Attachments are how a message carries something the reader must be able to see
	// regardless of their client: a remote <img> depends on the client fetching it, which
	// Outlook refuses by default and everyone refuses offline. Used for the personal-data
	// export and for feedback screenshots.
	SendEmailWithAttachments(toEmail, subject, htmlBody, textBody string, attachments []Attachment) error
}

// Attachment is one file travelling with an email.
type Attachment struct {
	Filename string
	MimeType string
	Content  []byte
}
