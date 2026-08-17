package notification

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/i18n"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification/provider/email"
	"github.com/rumi/rumi-be/internal/services/notification/provider/fcm"
	"github.com/rumi/rumi-be/internal/services/notification/provider/sms"
	"github.com/rumi/rumi-be/internal/templates"
	"go.uber.org/zap"
)

//go:embed locales/*.json
var localesFS embed.FS

type NotificationService struct {
	emailProvider email.Provider
	smsProvider   sms.Provider
	fcmProvider   fcm.Provider
	localizer     *i18n.Localizer
	logger        *zap.Logger
}

var GlobalNotificationService *NotificationService

func InitNotificationService(l *zap.Logger) {
	localizer, err := i18n.NewLocalizer(localesFS, "locales", "en-US")
	if err != nil {
		l.Fatal("Failed to initialize i18n localizer", zap.Error(err))
	}

	service := &NotificationService{
		emailProvider: email.NewMockProvider(l),
		smsProvider:   sms.NewMockProvider(l),
		fcmProvider:   fcm.NewMockProvider(l),
		localizer:     localizer,
		logger:        l,
	}

	// Email Provider Selection
	switch config.AppConfig.EmailProvider {
	case "sendgrid":
		if sg := email.NewSendGridProvider(l); sg != nil {
			l.Info("Configured SendGrid Email Provider")
			service.emailProvider = sg
		}
	case "smtp2go":
		if s2g := email.NewSMTP2GOProvider(l); s2g != nil {
			l.Info("Configured SMTP2GO Email Provider")
			service.emailProvider = s2g
		}
	default:
		l.Info("Using Mock Email Provider")
	}

	// SMS Provider Selection
	switch config.AppConfig.SMSProvider {
	case "twilio":
		if tw := sms.NewTwilioProvider(l); tw != nil {
			l.Info("Configured Twilio SMS Provider")
			service.smsProvider = tw
		}
	default:
		l.Info("Using Mock SMS Provider")
	}

	// FCM Provider Selection
	switch config.AppConfig.PushProvider {
	case "fcm":
		fcmProject := config.AppConfig.FirebaseProjectID
		if fcmProject == "" {
			fcmProject = config.AppConfig.GCPProjectID
		}
		fp, err := fcm.NewFCMProvider(context.Background(), l, fcmProject)
		if err != nil {
			l.Error("Failed to initialize FCM Push Provider, falling back to mock provider", zap.Error(err))
		} else {
			l.Info("Configured FCM Push Provider")
			service.fcmProvider = fp
		}
	default:
		l.Info("Using Mock FCM Push Provider")
	}

	GlobalNotificationService = service
}

func GetLocalizer() *i18n.Localizer {
	if GlobalNotificationService != nil {
		return GlobalNotificationService.localizer
	}
	return nil
}

func (n *NotificationService) SendVerificationEmail(email, code, locale string) error {
	vars := map[string]interface{}{
		"code": code,
		"year": time.Now().Year(),
	}
	subject := n.localizer.Get(locale, "verification_email_subject", vars)
	title := n.localizer.Get(locale, "verification_email_title", vars)
	instruction := n.localizer.Get(locale, "verification_email_instruction", vars)
	expiry := n.localizer.Get(locale, "verification_email_expiry", vars)
	ignore := n.localizer.Get(locale, "verification_email_ignore", vars)
	copyright := n.localizer.Get(locale, "copyright_notice", vars)
	footerText := n.localizer.Get(locale, "footer_text", vars)

	text := instruction + " " + code + "\n\n" + expiry + "\n" + ignore

	htmlContent, err := n.renderTemplate("verification_code.html", map[string]interface{}{
		"Subject":     subject,
		"Title":       title,
		"Instruction": instruction,
		"Code":        code,
		"Expiry":      expiry,
		"Ignore":      ignore,
		"Copyright":   copyright,
		"FooterText":  footerText,
	})
	if err != nil {
		n.logger.Warn("Failed to render HTML template, falling back to plain text", zap.Error(err))
		return n.emailProvider.SendEmailWithSender(config.AppConfig.EmailFromName, config.AppConfig.AuthEmailFromEmail, email, subject, text, text)
	}

	return n.emailProvider.SendEmailWithSender(config.AppConfig.EmailFromName, config.AppConfig.AuthEmailFromEmail, email, subject, htmlContent, text)
}

func (n *NotificationService) SendVerificationSMS(phone, code, locale string) error {
	vars := map[string]interface{}{"code": code}
	content := n.localizer.Get(locale, "verification_sms_content", vars)
	return n.smsProvider.SendSMS(phone, content)
}

func (n *NotificationService) SendWaitlistJoinedEmail(email, name, locale string) error {
	firstName := strings.TrimSpace(name)
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}

	vars := map[string]interface{}{
		"name":       firstName,
		"first_name": firstName,
		"year":       time.Now().Year(),
	}
	subject := n.localizer.Get(locale, "waitlist_joined_email_subject", vars)
	greeting := n.localizer.Get(locale, "greeting", vars)
	body1 := n.localizer.Get(locale, "waitlist_joined_email_body1", vars)
	body2 := n.localizer.Get(locale, "waitlist_joined_email_body2", vars)
	body3 := n.localizer.Get(locale, "waitlist_joined_email_body3", vars)
	closing := n.localizer.Get(locale, "waitlist_joined_email_closing", vars)
	if closing == "waitlist_joined_email_closing" {
		closing = "Armando"
	}
	team := n.localizer.Get(locale, "waitlist_joined_email_team", vars)
	if team == "waitlist_joined_email_team" {
		team = "Founder, rumi.coach"
	}
	copyright := n.localizer.Get(locale, "copyright_notice", vars)
	footerText := n.localizer.Get(locale, "footer_text", vars)

	// Try to render HTML template
	htmlContent, err := n.renderTemplate("waitlist_joined.html", map[string]interface{}{
		"Subject":    subject,
		"Greeting":   greeting,
		"BodyText1":  body1,
		"BodyText2":  body2,
		"BodyText3":  body3,
		"Closing":    closing,
		"Team":       team,
		"Copyright":  copyright,
		"FooterText": footerText,
	})

	text := greeting + "\n\n" + body1 + "\n\n" + body2 + "\n\n" + body3 + "\n\n" + closing + "\n" + team
	if err != nil {
		n.logger.Warn("Failed to render HTML template, falling back to plain text", zap.Error(err))
		return n.emailProvider.SendEmail(email, subject, text, text)
	}

	return n.emailProvider.SendEmail(email, subject, htmlContent, text)
}

func (n *NotificationService) SendAccountActivatedEmail(email, name, locale string) error {
	firstName := strings.TrimSpace(name)
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}

	vars := map[string]interface{}{
		"name":       firstName,
		"first_name": firstName,
		"year":       time.Now().Year(),
	}
	subject := n.localizer.Get(locale, "account_activated_email_subject", vars)
	greeting := n.localizer.Get(locale, "greeting", vars)
	body1 := n.localizer.Get(locale, "account_activated_email_body1", vars)
	body2 := n.localizer.Get(locale, "account_activated_email_body2", vars)
	body3 := n.localizer.Get(locale, "account_activated_email_body3", vars)
	ctaText := n.localizer.Get(locale, "account_activated_email_cta", vars)
	body4 := n.localizer.Get(locale, "account_activated_email_body4", vars)
	if body4 == "account_activated_email_body4" {
		body4 = "If anything feels off or confusing, just hit reply and tell me. I read every answer, and this is exactly the feedback we need right now."
	}
	closing := n.localizer.Get(locale, "account_activated_email_closing", vars)
	if closing == "account_activated_email_closing" {
		closing = "Armando"
	}
	team := n.localizer.Get(locale, "account_activated_email_team", vars)
	if team == "account_activated_email_team" {
		team = "Founder, rumi.coach"
	}
	copyright := n.localizer.Get(locale, "copyright_notice", vars)
	footerText := n.localizer.Get(locale, "footer_text", vars)

	// Try to render HTML template
	htmlContent, err := n.renderTemplate("account_active.html", map[string]interface{}{
		"Subject":    subject,
		"Greeting":   greeting,
		"BodyText1":  body1,
		"BodyText2":  body2,
		"BodyText3":  body3,
		"CtaText":    ctaText,
		"BodyText4":  body4,
		"LoginURL":   config.AppConfig.FrontendURL,
		"Closing":    closing,
		"Team":       team,
		"Copyright":  copyright,
		"FooterText": footerText,
	})

	text := greeting + "\n\n" + body1 + "\n\n" + body2 + "\n\n" + body3 + "\n\n" + ctaText + ": " + config.AppConfig.FrontendURL + "\n\n" + body4 + "\n\n" + closing + "\n" + team
	if err != nil {
		n.logger.Warn("Failed to render HTML template, falling back to plain text", zap.Error(err))
		return n.emailProvider.SendEmail(email, subject, text, text)
	}

	return n.emailProvider.SendEmail(email, subject, htmlContent, text)
}

func (n *NotificationService) SendWelcomeEmail(email, name, locale string) error {
	vars := map[string]interface{}{
		"name": name,
		"year": time.Now().Year(),
	}
	subject := n.localizer.Get(locale, "welcome_email_subject", vars)
	greeting := n.localizer.Get(locale, "greeting", vars)
	body1 := n.localizer.Get(locale, "welcome_email_body1", vars)
	body2 := n.localizer.Get(locale, "welcome_email_body2", vars)
	ctaText := n.localizer.Get(locale, "account_activated_email_cta", vars)
	body3 := n.localizer.Get(locale, "account_activated_email_body3", vars)
	closing := n.localizer.Get(locale, "closing_best", vars)
	team := n.localizer.Get(locale, "team_name", vars)
	copyright := n.localizer.Get(locale, "copyright_notice", vars)
	footerText := n.localizer.Get(locale, "footer_text", vars)

	// Try to render HTML template
	htmlContent, err := n.renderTemplate("welcome.html", map[string]interface{}{
		"Subject":    subject,
		"Greeting":   greeting,
		"BodyText1":  body1,
		"BodyText2":  body2,
		"CtaText":    ctaText,
		"LoginURL":   config.AppConfig.FrontendURL,
		"BodyText3":  body3,
		"Closing":    closing,
		"Team":       team,
		"Copyright":  copyright,
		"FooterText": footerText,
	})

	text := greeting + "\n\n" + body1 + "\n\n" + body2 + "\n\n" + ctaText + ": " + config.AppConfig.FrontendURL + "\n\n" + body3 + "\n\n" + closing + "\n" + team
	if err != nil {
		n.logger.Warn("Failed to render HTML template, falling back to plain text", zap.Error(err))
		return n.emailProvider.SendEmail(email, subject, text, text)
	}

	return n.emailProvider.SendEmail(email, subject, htmlContent, text)
}

func (n *NotificationService) SendAccountActivatedSMS(phone, name, locale string) error {
	vars := map[string]interface{}{"name": name}
	content := n.localizer.Get(locale, "account_activated_sms", vars)
	return n.smsProvider.SendSMS(phone, content)
}

func (n *NotificationService) SendWaitlistJoinedSMS(phone, name, locale string) error {
	vars := map[string]interface{}{"name": name}
	content := n.localizer.Get(locale, "waitlist_joined_sms", vars)
	return n.smsProvider.SendSMS(phone, content)
}

func (n *NotificationService) SendWelcomeSMS(phone, name, locale string) error {
	vars := map[string]interface{}{"name": name}
	content := n.localizer.Get(locale, "welcome_sms", vars)
	return n.smsProvider.SendSMS(phone, content)
}

func (n *NotificationService) renderTemplate(tmplName string, data interface{}) (string, error) {
	// Templates are embedded in the binary (internal/templates), so rendering
	// works regardless of the process working directory or deployment image.
	tmpl, err := template.ParseFS(templates.Emails, "emails/"+tmplName)
	if err != nil {
		return "", err
	}

	var dataMap map[string]interface{}
	if m, ok := data.(map[string]interface{}); ok {
		dataMap = m
	} else {
		dataMap = make(map[string]interface{})
	}
	dataMap["FrontendURL"] = config.AppConfig.FrontendURL

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, dataMap); err != nil {
		return "", err
	}

	return buf.String(), nil
}

type RecommendationItem struct {
	Title       string  `json:"title"`
	Type        string  `json:"type"` // book, article, video, podcast, other
	Author      *string `json:"author,omitempty"`
	URL         *string `json:"url,omitempty"`
	Description string  `json:"description"`
}

type RecommendationTemplateItem struct {
	Title       string
	Type        string
	TypeIcon    string
	Author      string
	URL         string
	Description string
}

func (n *NotificationService) SendRecommendationsEmail(email, name string, items []RecommendationItem, locale string) error {
	var subject, greeting, intro, closing, team, copyright, footerText string

	vars := map[string]interface{}{
		"name": name,
		"year": time.Now().Year(),
	}
	subject = n.localizer.Get(locale, "recommendations_email_subject", vars)
	greeting = n.localizer.Get(locale, "greeting", vars)
	intro = n.localizer.Get(locale, "recommendations_email_body1", vars)
	closing = n.localizer.Get(locale, "closing_best", vars)
	team = n.localizer.Get(locale, "team_name", vars)
	copyright = n.localizer.Get(locale, "copyright_notice", vars)
	footerText = n.localizer.Get(locale, "footer_text", vars)

	// 2. Format and translate recommendation items
	tmplItems := make([]RecommendationTemplateItem, len(items))
	for i, item := range items {
		var formattedType, typeIcon string

		// Resolve localized Type and Icon
		switch item.Type {
		case "book":
			typeIcon = "📚"
			if locale == "pt-PT" || locale == "pt-BR" {
				formattedType = "Livro"
			} else {
				formattedType = "Book"
			}
		case "article":
			typeIcon = "📄"
			if locale == "pt-PT" || locale == "pt-BR" {
				formattedType = "Artigo"
			} else {
				formattedType = "Article"
			}
		case "video":
			typeIcon = "🎥"
			if locale == "pt-PT" || locale == "pt-BR" {
				formattedType = "Vídeo"
			} else {
				formattedType = "Video"
			}
		case "podcast":
			typeIcon = "🎙"
			if locale == "pt-PT" || locale == "pt-BR" {
				formattedType = "Podcast"
			} else {
				formattedType = "Podcast"
			}
		default:
			typeIcon = "🔗"
			if locale == "pt-PT" || locale == "pt-BR" {
				formattedType = "Recurso"
			} else {
				formattedType = "Resource"
			}
		}

		authorVal := ""
		if item.Author != nil {
			authorVal = *item.Author
			// Prefix author label dynamically if it doesn't already contain it
			if locale == "pt-PT" || locale == "pt-BR" {
				authorVal = "Por " + authorVal
			} else {
				authorVal = "By " + authorVal
			}
		}

		urlVal := ""
		if item.URL != nil && *item.URL != "" {
			urlVal = appendUTM(*item.URL, "rumi_coach", "email", "ai_recommendations")
		}

		tmplItems[i] = RecommendationTemplateItem{
			Title:       item.Title,
			Type:        formattedType,
			TypeIcon:    typeIcon,
			Author:      authorVal,
			URL:         urlVal,
			Description: item.Description,
		}
	}

	// 3. Render HTML template
	htmlContent, err := n.renderTemplate("recommendations.html", map[string]interface{}{
		"Subject":         subject,
		"Greeting":        greeting,
		"Intro":           intro,
		"Recommendations": tmplItems,
		"Locale":          locale,
		"LoginURL":        config.AppConfig.FrontendURL + "/login",
		"Closing":         closing,
		"Team":            team,
		"Copyright":       copyright,
		"FooterText":      footerText,
	})

	// Plain-text alternative (also the fallback when the template fails)
	var buf bytes.Buffer
	buf.WriteString(greeting + "\n\n" + intro + "\n\n")
	for _, item := range tmplItems {
		buf.WriteString(fmt.Sprintf("%s %s: %s\n", item.TypeIcon, item.Type, item.Title))
		if item.Author != "" {
			buf.WriteString(item.Author + "\n")
		}
		buf.WriteString(item.Description + "\n")
		if item.URL != "" {
			buf.WriteString(item.URL + "\n")
		}
		buf.WriteString("\n")
	}
	buf.WriteString(closing + "\n" + team)

	if err != nil {
		n.logger.Warn("Failed to render HTML template for recommendations, falling back to plain text", zap.Error(err))
		return n.emailProvider.SendEmail(email, subject, buf.String(), buf.String())
	}

	return n.emailProvider.SendEmail(email, subject, htmlContent, buf.String())
}

func appendUTM(rawURL, source, medium, campaign string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set("utm_source", source)
	q.Set("utm_medium", medium)
	q.Set("utm_campaign", campaign)
	u.RawQuery = q.Encode()
	return u.String()
}

func (n *NotificationService) SendPushNotification(ctx context.Context, userID, title, body string) error {
	var devices []models.UserDevice
	if err := database.DB.Where("user_id = ?", userID).Find(&devices).Error; err != nil {
		return fmt.Errorf("failed to fetch user devices for push notification: %w", err)
	}

	if len(devices) == 0 {
		n.logger.Info("no registered devices found for user, skipping push notification", zap.String("user_id", userID))
		return nil
	}

	var lastErr error
	for _, dev := range devices {
		if err := n.fcmProvider.SendPush(ctx, dev.FCMToken, title, body); err != nil {
			n.logger.Error("failed to send push notification to device",
				zap.String("user_id", userID),
				zap.String("device_id", dev.ID),
				zap.Error(err))
			lastErr = err
		}
	}

	return lastErr
}

func (n *NotificationService) SendLeadNotificationEmail(lead *models.Lead) error {
	subject := "New Business Lead: " + lead.Company

	phone := "N/A"
	if lead.Phone != nil && *lead.Phone != "" {
		phone = *lead.Phone
	}

	message := "N/A"
	if lead.Message != nil && *lead.Message != "" {
		message = *lead.Message
	}

	origin := "N/A"
	if lead.Origin != nil && *lead.Origin != "" {
		origin = *lead.Origin
	}

	country := "N/A"
	if lead.Country != nil && *lead.Country != "" {
		country = *lead.Country
	}

	size := "N/A"
	if lead.Size != nil && *lead.Size != "" {
		size = *lead.Size
	}

	data := map[string]interface{}{
		"Name":    lead.Name,
		"Email":   lead.Email,
		"Phone":   phone,
		"Company": lead.Company,
		"Country": country,
		"Size":    size,
		"Message": message,
		"Origin":  origin,
	}

	htmlContent, err := n.renderTemplate("lead_notification.html", data)
	if err != nil {
		n.logger.Error("Failed to render lead_notification.html template", zap.Error(err))
		return err
	}

	body := "A new lead has been submitted.\n\n" +
		"Name: " + lead.Name + "\n" +
		"Email: " + lead.Email + "\n" +
		"Phone: " + phone + "\n" +
		"Company: " + lead.Company + "\n" +
		"Size: " + size + "\n" +
		"Country: " + country + "\n" +
		"Message: " + message + "\n" +
		"Origin: " + origin + "\n\n" +
		"Please reach out to them as soon as possible."

	return n.emailProvider.SendEmail("sales@rumi.coach", subject, htmlContent, body)
}

func (n *NotificationService) SendLeadConfirmationEmail(lead *models.Lead) error {
	locale := "en-US"
	if lead.Language == "pt" || lead.Language == "pt-PT" || lead.Language == "pt-BR" {
		locale = "pt-PT"
	}
	if lead.Language != "" && len(lead.Language) > 2 {
		locale = lead.Language
	}

	vars := map[string]interface{}{"name": lead.Name}
	subject := n.localizer.Get(locale, "lead_confirmation_subject", vars)
	greeting := n.localizer.Get(locale, "greeting", vars)
	bodyText1 := n.localizer.Get(locale, "lead_confirmation_body1", vars)
	bodyText2 := n.localizer.Get(locale, "lead_confirmation_body2", vars)
	closing := n.localizer.Get(locale, "closing_regards", vars)
	team := n.localizer.Get(locale, "team_name", vars)
	footerText := n.localizer.Get(locale, "lead_confirmation_footer", vars)

	year := time.Now().Year()

	data := map[string]interface{}{
		"Subject":    subject,
		"Greeting":   greeting,
		"BodyText1":  bodyText1,
		"BodyText2":  bodyText2,
		"BodyText3":  "",
		"Closing":    closing,
		"Team":       team,
		"FooterText": footerText,
		"Year":       year,
	}

	htmlContent, err := n.renderTemplate("lead_confirmation.html", data)
	if err != nil {
		n.logger.Error("Failed to render lead_confirmation.html template", zap.Error(err))
		return err
	}

	textBody := greeting + "\n\n" +
		bodyText1 + "\n\n" +
		bodyText2 + "\n\n" +
		closing + "\n" + team

	return n.emailProvider.SendEmailWithSender("Rumi for Business", "sales@rumi.coach", lead.Email, subject, htmlContent, textBody)
}

// dataExportMaxBytes is the point past which the export stops being emailable. Providers
// cap attachments around 10–25MB and reject the whole message beyond it, so a silent
// failure is the realistic outcome; better to log it loudly and let the caller decide.
const dataExportMaxBytes = 8 * 1024 * 1024

// SendDataExportEmail delivers the user's personal-data export as a JSON attachment.
// It exists because the download route is close to useless on mobile, where a browser
// download has nowhere meaningful to land — the user asks in the app and the file arrives
// where they can actually keep it.
//
// The attachment is the export itself; the body never quotes any of it.
func (n *NotificationService) SendDataExportEmail(toEmail, name, locale string, payload []byte) error {
	if toEmail == "" {
		return fmt.Errorf("no email address on file")
	}
	if len(payload) > dataExportMaxBytes {
		n.logger.Error("data export too large to email",
			zap.Int("bytes", len(payload)), zap.Int("limit", dataExportMaxBytes))
		return fmt.Errorf("export is %d bytes, over the %d byte email limit", len(payload), dataExportMaxBytes)
	}

	firstName := name
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}

	vars := map[string]interface{}{
		"name":       firstName,
		"first_name": firstName,
		"year":       time.Now().Year(),
	}

	subject := n.localizer.Get(locale, "data_export_email_subject", vars)
	if subject == "data_export_email_subject" {
		subject = "Your Rumi data"
	}
	greeting := n.localizer.Get(locale, "greeting", vars)
	body1 := n.localizer.Get(locale, "data_export_email_body1", vars)
	if body1 == "data_export_email_body1" {
		body1 = "You asked for a copy of your data, and it is attached to this email as a JSON file."
	}
	body2 := n.localizer.Get(locale, "data_export_email_body2", vars)
	if body2 == "data_export_email_body2" {
		body2 = "It contains what you have shared with Rumi and what we hold about you: your profile, your memories, your commitments, your sessions and your minutes."
	}
	body3 := n.localizer.Get(locale, "data_export_email_body3", vars)
	if body3 == "data_export_email_body3" {
		body3 = "Keep the file somewhere safe — anyone who opens it can read it. If you did not request this, please reply and tell us."
	}
	closing := n.localizer.Get(locale, "data_export_email_closing", vars)
	if closing == "data_export_email_closing" {
		closing = "Rumi"
	}
	team := n.localizer.Get(locale, "data_export_email_team", vars)
	if team == "data_export_email_team" {
		team = "rumi.coach"
	}
	copyright := n.localizer.Get(locale, "copyright_notice", vars)
	footerText := n.localizer.Get(locale, "footer_text", vars)

	text := greeting + "\n\n" + body1 + "\n\n" + body2 + "\n\n" + body3 + "\n\n" + closing + "\n" + team

	htmlContent, err := n.renderTemplate("data_export.html", map[string]interface{}{
		"Subject":    subject,
		"Greeting":   greeting,
		"BodyText1":  body1,
		"BodyText2":  body2,
		"BodyText3":  body3,
		"LoginURL":   config.AppConfig.FrontendURL,
		"Closing":    closing,
		"Team":       team,
		"Copyright":  copyright,
		"FooterText": footerText,
	})
	if err != nil {
		n.logger.Warn("Failed to render data export template, falling back to plain text", zap.Error(err))
		htmlContent = text
	}

	return n.emailProvider.SendEmailWithAttachments(toEmail, subject, htmlContent, text,
		[]email.Attachment{{
			Filename: "rumicoach_export.json",
			MimeType: "application/json",
			Content:  payload,
		}})
}

// feedbackRecipient routes a report to the people who act on it. Bugs and product ideas
// want different eyes and different urgency; sending both to one inbox means one of the
// two is skimmed past.
func feedbackRecipient(category string) string {
	if category == models.FeedbackCategoryBug {
		if addr := config.AppConfig.FeedbackEmailBugs; addr != "" {
			return addr
		}
		return "support@rumi.coach"
	}
	if addr := config.AppConfig.FeedbackEmail; addr != "" {
		return addr
	}
	return "feedback@rumi.coach"
}

// labelColors give each category its own badge, so the inbox can be triaged at a glance
// without reading a word.
var feedbackLabelColors = map[string]string{
	models.FeedbackCategoryBug:     "#B3261E",
	models.FeedbackCategoryFeature: "#1A5F4F",
	models.FeedbackCategoryGeneral: "#7A6A55",
}

// SendFeedbackNotificationEmail tells the team about a report from the app.
//
// Both an HTML and a plain-text body are built, and they are genuinely different documents.
// Sending the same string as both meant the text version — newlines and all — was rendered
// as HTML, which collapses whitespace: the report arrived as one unbroken paragraph with
// the diagnostics run together at the end.
//
// Screenshots are embedded rather than merely linked. A bug is usually obvious the moment
// you see it, and making somebody click through is the difference between a report that
// gets triaged and one that does not.
//
// Deliberately not localized: this one is read by the team, not by the user.
func (n *NotificationService) SendFeedbackNotificationEmail(category, description string, context [][2]string, imageLinks []string, images []email.Attachment) error {
	label := map[string]string{
		models.FeedbackCategoryBug:     "Bug report",
		models.FeedbackCategoryFeature: "Feature request",
		models.FeedbackCategoryGeneral: "Feedback",
	}[category]
	if label == "" {
		label = "Feedback"
	}
	color := feedbackLabelColors[category]
	if color == "" {
		color = "#7A6A55"
	}

	// Blank diagnostics are dropped here rather than in the template: a row reading
	// "Device:" with nothing after it is worse than no row.
	rows := make([][2]string, 0, len(context))
	for _, kv := range context {
		if strings.TrimSpace(kv[1]) != "" {
			rows = append(rows, kv)
		}
	}

	var text strings.Builder
	text.WriteString(label + "\n\n")
	text.WriteString(description + "\n\n")
	text.WriteString("---\n")
	for _, kv := range rows {
		fmt.Fprintf(&text, "%s: %s\n", kv[0], kv[1])
	}
	if len(imageLinks) > 0 {
		text.WriteString("\nScreenshots (links expire):\n")
		for _, link := range imageLinks {
			text.WriteString(link + "\n")
		}
	}

	subject := fmt.Sprintf("[%s] %s", label, firstLine(description, 60))
	recipient := feedbackRecipient(category)

	htmlContent, err := n.renderTemplate("feedback_notification.html", map[string]interface{}{
		"Label":       label,
		"LabelColor":  color,
		"Description": description,
		"Context":     rows,
		"Images":      imageLinks,
	})
	if err != nil {
		n.logger.Warn("Failed to render feedback template, falling back to plain text", zap.Error(err))
		htmlContent = text.String()
	}

	// Screenshots travel WITH the message, not only as links in it. The links are still
	// there and still work, but a remote <img> only appears if the client agrees to fetch
	// it — Outlook refuses by default, everyone refuses offline, and the link expires after
	// two weeks. An attachment is visible in every client, forever, which matters because
	// seeing the bug is what gets it triaged.
	//
	// Oversized sets fall back to links alone: providers reject the whole message past a
	// limit, and a report that never arrives is worse than one without its screenshot.
	total := 0
	for _, img := range images {
		total += len(img.Content)
	}
	if len(images) == 0 || total > feedbackAttachmentBudget {
		if total > feedbackAttachmentBudget {
			n.logger.Info("feedback screenshots too large to attach; sending links only",
				zap.Int("bytes", total), zap.Int("budget", feedbackAttachmentBudget))
		}
		return n.emailProvider.SendEmail(recipient, subject, htmlContent, text.String())
	}
	return n.emailProvider.SendEmailWithAttachments(recipient, subject, htmlContent, text.String(), images)
}

// feedbackAttachmentBudget is the total attachment size past which screenshots are linked
// rather than attached. Well under the ~25MB most providers accept, because the whole
// message is rejected when it is exceeded, not just the offending file.
const feedbackAttachmentBudget = 12 << 20

// firstLine is the report's opening, trimmed to fit a subject line.
func firstLine(s string, max int) string {
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
