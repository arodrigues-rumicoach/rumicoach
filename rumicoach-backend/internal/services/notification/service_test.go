package notification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/i18n"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification/provider/email"
	"github.com/rumi/rumi-be/internal/services/notification/provider/fcm"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRenderTemplates(t *testing.T) {
	// Initialize config
	config.AppConfig = &config.Config{
		FrontendURL: "http://localhost:5173",
	}

	logger, _ := zap.NewDevelopment()
	svc := &NotificationService{
		logger: logger,
	}

	// Change cwd to repo root to resolve relative template directory correctly during go test
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalCwd)
	}()

	// Go upward until we find go.mod to ensure we're at repo root
	dir := originalCwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	_ = os.Chdir(dir)

	// Test data map
	testData := map[string]interface{}{
		"Subject":    "Test Subject",
		"Greeting":   "Hello User,",
		"BodyText1":  "This is body text 1.",
		"BodyText2":  "This is body text 2.",
		"BodyText3":  "This is body text 3.",
		"CtaText":    "Commitment Link",
		"LoginURL":   config.AppConfig.FrontendURL + "/login",
		"Closing":    "Best regards,",
		"Team":       "Rumi Team",
		"Copyright":  "© 2026 Rumi",
		"FooterText": "Some footer text",
		// verification_code.html
		"Title":       "Verification code",
		"Instruction": "Use this code to confirm your email address:",
		"Code":        "482913",
		"Expiry":      "This code is valid for 15 minutes.",
		"Ignore":      "If you didn't request this code, you can safely ignore this email.",
		"Recommendations": []map[string]string{
			{
				"Title":       "Test Book",
				"Type":        "Book",
				"TypeIcon":    "📚",
				"Author":      "By Test Author",
				"Description": "This is a test book recommendation.",
				"URL":         "https://example.com/book",
			},
		},
	}

	templates := []string{
		"account_active.html",
		"welcome.html",
		"waitlist_joined.html",
		"recommendations.html",
		"verification_code.html",
	}

	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			rendered, err := svc.renderTemplate(tmpl, testData)
			if err != nil {
				t.Fatalf("Failed to render template %s: %v", tmpl, err)
			}

			if rendered == "" {
				t.Errorf("Rendered content is empty for %s", tmpl)
			}

			// Verify general structural content
			lowerRendered := strings.ToLower(rendered)
			if !strings.Contains(lowerRendered, "rumi") || !strings.Contains(lowerRendered, "coach") {
				t.Errorf("Expected template %s to contain branding 'rumi' and 'coach'", tmpl)
			}

			switch tmpl {
			case "waitlist_joined.html", "recommendations.html":
				// No CTA button in these templates.
			case "verification_code.html":
				if !strings.Contains(rendered, "482913") {
					t.Errorf("Expected template %s to contain the verification code", tmpl)
				}
			default:
				// Verify button/URL rendering for templates that have CTA
				if !strings.Contains(rendered, "http://localhost:5173") {
					t.Errorf("Expected template %s to contain correct LoginURL 'http://localhost:5173', got: %s", tmpl, rendered)
				}
				if !strings.Contains(rendered, "Commitment Link") {
					t.Errorf("Expected template %s to contain CtaText 'Commitment Link'", tmpl)
				}
			}
		})
	}
}

func TestSendPushNotification(t *testing.T) {
	// Initialize in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	database.DB = db

	err = db.Exec(`CREATE TABLE user_devices (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		fcm_token TEXT UNIQUE NOT NULL,
		platform TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`).Error
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	logger := zap.NewNop()
	mockFCM := fcm.NewMockProvider(logger)

	svc := &NotificationService{
		fcmProvider: mockFCM,
		logger:      logger,
	}

	userID := "user-789"

	// 1. Send push with no devices registered
	err = svc.SendPushNotification(context.Background(), userID, "Hello", "World")
	if err != nil {
		t.Errorf("Expected no error when no devices registered, got: %v", err)
	}

	// 2. Register devices
	db.Create(&models.UserDevice{
		UserID:   userID,
		FCMToken: "token-1",
		Platform: "ios",
	})
	db.Create(&models.UserDevice{
		UserID:   userID,
		FCMToken: "token-2",
		Platform: "android",
	})

	err = svc.SendPushNotification(context.Background(), userID, "Test Title", "Test Body")
	if err != nil {
		t.Errorf("Expected no error when sending to registered devices, got: %v", err)
	}
}

func TestSendWaitlistJoinedEmail(t *testing.T) {
	logger := zap.NewNop()
	mockEmail := &recordingEmailProvider{}
	loc, err := i18n.NewLocalizer(localesFS, "locales", "en-US")
	if err != nil {
		t.Fatalf("Failed to create localizer: %v", err)
	}

	svc := &NotificationService{
		emailProvider: mockEmail,
		localizer:     loc,
		logger:        logger,
	}

	err = svc.SendWaitlistJoinedEmail("user@example.com", "John Doe", "en-US")
	if err != nil {
		t.Fatalf("SendWaitlistJoinedEmail failed: %v", err)
	}

	if !strings.Contains(mockEmail.lastText, "Hi John,") {
		t.Errorf("Expected email greeting 'Hi John,', got text: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "Welcome, and thank you for joining the Rumi waitlist.") {
		t.Errorf("Expected body1 in text, got: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "Armando\nFounder, rumi.coach") {
		t.Errorf("Expected signature in text, got: %s", mockEmail.lastText)
	}
}

func TestSendAccountActivatedEmail(t *testing.T) {
	config.AppConfig = &config.Config{
		FrontendURL: "http://localhost:5173",
	}

	logger := zap.NewNop()
	mockEmail := &recordingEmailProvider{}
	loc, err := i18n.NewLocalizer(localesFS, "locales", "en-US")
	if err != nil {
		t.Fatalf("Failed to create localizer: %v", err)
	}

	svc := &NotificationService{
		emailProvider: mockEmail,
		localizer:     loc,
		logger:        logger,
	}

	err = svc.SendAccountActivatedEmail("user@example.com", "Jane Smith", "en-US")
	if err != nil {
		t.Fatalf("SendAccountActivatedEmail failed: %v", err)
	}

	if mockEmail.lastSubject != "Your spot is ready" {
		t.Errorf("Expected subject 'Your spot is ready', got: %s", mockEmail.lastSubject)
	}
	if !strings.Contains(mockEmail.lastText, "Hi Jane,") {
		t.Errorf("Expected email greeting 'Hi Jane,', got text: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "Your spot is ready.") {
		t.Errorf("Expected body1 in text, got: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "Start your first session: http://localhost:5173") {
		t.Errorf("Expected CTA link in text, got: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "If anything feels off or confusing, just hit reply and tell me.") {
		t.Errorf("Expected body4 in text, got: %s", mockEmail.lastText)
	}
	if !strings.Contains(mockEmail.lastText, "Armando\nFounder, rumi.coach") {
		t.Errorf("Expected signature in text, got: %s", mockEmail.lastText)
	}
}

type recordingEmailProvider struct {
	lastTo          string
	lastSubject     string
	lastHTML        string
	lastText        string
	lastFilename    string
	lastMimeType    string
	lastAttachment  []byte
	lastAttachments []email.Attachment
}

func (r *recordingEmailProvider) SendEmail(toEmail, subject, htmlBody, textBody string) error {
	r.lastTo = toEmail
	r.lastSubject = subject
	r.lastHTML = htmlBody
	r.lastText = textBody
	return nil
}

func (r *recordingEmailProvider) SendEmailWithSender(fromName, fromEmail, toEmail, subject, htmlBody, textBody string) error {
	return r.SendEmail(toEmail, subject, htmlBody, textBody)
}

func (r *recordingEmailProvider) SendEmailWithAttachments(toEmail, subject, htmlBody, textBody string, attachments []email.Attachment) error {
	r.lastAttachments = attachments
	if len(attachments) > 0 {
		r.lastFilename = attachments[0].Filename
		r.lastMimeType = attachments[0].MimeType
		r.lastAttachment = attachments[0].Content
	}
	return r.SendEmail(toEmail, subject, htmlBody, textBody)
}

// The export is the user's own data leaving the system. What matters is that it goes out
// as an attachment (not quoted into the body), addressed to them, in their language — and
// that an account with nowhere to send it fails instead of silently dropping the request.
func TestSendDataExportEmail(t *testing.T) {
	config.AppConfig = &config.Config{FrontendURL: "http://localhost:5173"}
	mockEmail := &recordingEmailProvider{}
	loc, err := i18n.NewLocalizer(localesFS, "locales", "en-US")
	if err != nil {
		t.Fatalf("Failed to create localizer: %v", err)
	}
	svc := &NotificationService{emailProvider: mockEmail, localizer: loc, logger: zap.NewNop()}

	payload := []byte(`{"user":{"name":"Armando"},"memories":[]}`)
	if err := svc.SendDataExportEmail("user@example.com", "Armando Rodrigues", "en-US", payload); err != nil {
		t.Fatalf("SendDataExportEmail failed: %v", err)
	}

	if mockEmail.lastTo != "user@example.com" {
		t.Errorf("sent to %q", mockEmail.lastTo)
	}
	if string(mockEmail.lastAttachment) != string(payload) {
		t.Errorf("the attachment must be the export verbatim, got %q", mockEmail.lastAttachment)
	}
	if mockEmail.lastFilename != "rumicoach_export.json" || mockEmail.lastMimeType != "application/json" {
		t.Errorf("attachment metadata = %q / %q", mockEmail.lastFilename, mockEmail.lastMimeType)
	}
	// The body must not carry the data. It is attached; quoting it into the message would
	// put personal data in the preview pane of every device the mailbox syncs to.
	if strings.Contains(mockEmail.lastHTML, "Armando\",") || strings.Contains(mockEmail.lastText, `"memories"`) {
		t.Error("the export content leaked into the email body")
	}
	// Localized subject, not the raw key.
	if mockEmail.lastSubject == "" || strings.Contains(mockEmail.lastSubject, "data_export_email_subject") {
		t.Errorf("subject not localized: %q", mockEmail.lastSubject)
	}

	// Portuguese must be Portuguese — the keys exist in every locale precisely so the
	// fallback never has to fire.
	if err := svc.SendDataExportEmail("user@example.com", "Armando", "pt-PT", payload); err != nil {
		t.Fatalf("pt-PT send failed: %v", err)
	}
	if !strings.Contains(mockEmail.lastText, "dados") {
		t.Errorf("pt-PT body fell back to English: %q", mockEmail.lastText)
	}
}

func TestSendDataExportEmailRefusesTheImpossible(t *testing.T) {
	config.AppConfig = &config.Config{FrontendURL: "http://localhost:5173"}
	loc, err := i18n.NewLocalizer(localesFS, "locales", "en-US")
	if err != nil {
		t.Fatalf("Failed to create localizer: %v", err)
	}
	svc := &NotificationService{emailProvider: &recordingEmailProvider{}, localizer: loc, logger: zap.NewNop()}

	if err := svc.SendDataExportEmail("", "Armando", "en-US", []byte("{}")); err == nil {
		t.Error("an account with no address must fail, not report success")
	}
	// Providers reject oversized attachments outright, so failing here is the difference
	// between a logged error and a user waiting forever for an email nobody sent.
	huge := make([]byte, dataExportMaxBytes+1)
	if err := svc.SendDataExportEmail("user@example.com", "Armando", "en-US", huge); err == nil {
		t.Error("an export past the attachment limit must fail loudly")
	}
}

// Bugs and product ideas go to different people. Sending both to one inbox means one of
// the two gets skimmed past, which is how feature requests die.
func TestFeedbackRoutingByCategory(t *testing.T) {
	config.AppConfig = &config.Config{}
	if got := feedbackRecipient(models.FeedbackCategoryBug); got != "support@rumi.coach" {
		t.Errorf("bug reports go to support, got %q", got)
	}
	for _, c := range []string{models.FeedbackCategoryGeneral, models.FeedbackCategoryFeature} {
		if got := feedbackRecipient(c); got != "feedback@rumi.coach" {
			t.Errorf("%s should go to feedback@, got %q", c, got)
		}
	}

	// Configuration overrides both, independently.
	config.AppConfig = &config.Config{FeedbackEmail: "ideas@x.com", FeedbackEmailBugs: "bugs@x.com"}
	if got := feedbackRecipient(models.FeedbackCategoryBug); got != "bugs@x.com" {
		t.Errorf("bug override = %q", got)
	}
	if got := feedbackRecipient(models.FeedbackCategoryFeature); got != "ideas@x.com" {
		t.Errorf("feature override = %q", got)
	}
}

func TestSendFeedbackNotificationEmail(t *testing.T) {
	config.AppConfig = &config.Config{}
	mockEmail := &recordingEmailProvider{}
	svc := &NotificationService{emailProvider: mockEmail, logger: zap.NewNop()}

	err := svc.SendFeedbackNotificationEmail(
		models.FeedbackCategoryBug,
		"The wheel screen is blank\nafter finishing a session.",
		[][2]string{{"User", "u1"}, {"Platform", "ios"}, {"App version", "1.4.2"}, {"Device", ""}},
		[]string{"https://signed/one.png"}, nil,
	)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if mockEmail.lastTo != "support@rumi.coach" {
		t.Errorf("bug report went to %q", mockEmail.lastTo)
	}
	// The subject is the first line only, so a multi-line report does not wrap the inbox.
	if !strings.HasPrefix(mockEmail.lastSubject, "[Bug report] The wheel screen is blank") ||
		strings.Contains(mockEmail.lastSubject, "after finishing") {
		t.Errorf("subject = %q", mockEmail.lastSubject)
	}
	for _, want := range []string{"ios", "1.4.2", "https://signed/one.png", "after finishing a session."} {
		if !strings.Contains(mockEmail.lastText, want) {
			t.Errorf("body is missing %q:\n%s", want, mockEmail.lastText)
		}
	}
	// Empty fields are skipped rather than printed as "Device: ".
	if strings.Contains(mockEmail.lastText, "Device:") {
		t.Errorf("blank diagnostics should be omitted:\n%s", mockEmail.lastText)
	}
}

// The first version sent the plain-text body as the HTML part too. HTML collapses newlines,
// so the report arrived as one unbroken paragraph with the diagnostics run together at the
// end — which is what a real inbox showed. The two parts are different documents.
func TestFeedbackEmailHTMLAndTextAreDifferentDocuments(t *testing.T) {
	config.AppConfig = &config.Config{}
	mockEmail := &recordingEmailProvider{}
	svc := &NotificationService{emailProvider: mockEmail, logger: zap.NewNop()}

	err := svc.SendFeedbackNotificationEmail(
		models.FeedbackCategoryBug,
		"First line of the report.\nSecond line, which must not run into the first.",
		[][2]string{{"User", "u1"}, {"Platform", "web"}, {"Device", ""}},
		[]string{"https://signed/shot.png"}, nil,
	)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if mockEmail.lastHTML == mockEmail.lastText {
		t.Fatal("the HTML part is still the plain-text body; newlines will collapse")
	}
	if !strings.Contains(mockEmail.lastHTML, "<html") {
		t.Errorf("the HTML part is not HTML:\n%s", mockEmail.lastHTML)
	}
	// The template must preserve the author's line breaks rather than reflow them.
	if !strings.Contains(mockEmail.lastHTML, "white-space: pre-wrap") {
		t.Error("line breaks in the report would be collapsed by the renderer")
	}
	// Screenshots are shown, not just linked: seeing the bug is the point.
	if !strings.Contains(mockEmail.lastHTML, `<img src="https://signed/shot.png"`) {
		t.Errorf("the screenshot is not embedded:\n%s", mockEmail.lastHTML)
	}
	// Diagnostics survive to both parts.
	for _, part := range []string{mockEmail.lastHTML, mockEmail.lastText} {
		if !strings.Contains(part, "web") {
			t.Errorf("diagnostics missing from a part:\n%s", part)
		}
	}
	// A blank value is dropped rather than printed as a label with nothing after it.
	if strings.Contains(mockEmail.lastHTML, ">Device<") || strings.Contains(mockEmail.lastText, "Device:") {
		t.Error("an empty diagnostic was rendered as an empty row")
	}
	// And the text alternative is still plain text.
	if strings.Contains(mockEmail.lastText, "<html") {
		t.Error("the text part contains markup")
	}
}

// The report arrived with no images because a remote <img> is only as reliable as the
// reader's client: Outlook blocks remote content by default, everyone blocks it offline,
// and the signed link expires after two weeks. Attaching the bytes is what makes the
// screenshot actually arrive.
func TestFeedbackScreenshotsTravelWithTheEmail(t *testing.T) {
	config.AppConfig = &config.Config{}
	mockEmail := &recordingEmailProvider{}
	svc := &NotificationService{emailProvider: mockEmail, logger: zap.NewNop()}

	shot := []byte("not really a png, but bytes are bytes")
	err := svc.SendFeedbackNotificationEmail(
		models.FeedbackCategoryBug, "blank screen",
		[][2]string{{"User", "u1"}},
		[]string{"https://signed/one.png"},
		[]email.Attachment{{Filename: "screenshot-1.png", MimeType: "image/png", Content: shot}},
	)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mockEmail.lastAttachments) != 1 {
		t.Fatalf("the screenshot was not attached: %d attachments", len(mockEmail.lastAttachments))
	}
	if string(mockEmail.lastAttachments[0].Content) != string(shot) {
		t.Error("the attached bytes are not the screenshot")
	}
	// The link stays too — it is the full-size copy in the bucket.
	if !strings.Contains(mockEmail.lastHTML, "https://signed/one.png") {
		t.Error("the signed link should survive alongside the attachment")
	}
}

// Providers reject the entire message past a size limit, so an oversized set falls back to
// links. A report that never arrives is worse than one without its screenshots.
func TestOversizedFeedbackScreenshotsFallBackToLinks(t *testing.T) {
	config.AppConfig = &config.Config{}
	mockEmail := &recordingEmailProvider{}
	svc := &NotificationService{emailProvider: mockEmail, logger: zap.NewNop()}

	huge := email.Attachment{
		Filename: "big.png", MimeType: "image/png",
		Content: make([]byte, feedbackAttachmentBudget+1),
	}
	if err := svc.SendFeedbackNotificationEmail(
		models.FeedbackCategoryBug, "x", [][2]string{{"User", "u1"}},
		[]string{"https://signed/big.png"}, []email.Attachment{huge},
	); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(mockEmail.lastAttachments) != 0 {
		t.Error("an oversized set must not be attached")
	}
	// But the report still went out, with the link.
	if !strings.Contains(mockEmail.lastHTML, "https://signed/big.png") {
		t.Error("the report should still have been sent, with links")
	}
}
