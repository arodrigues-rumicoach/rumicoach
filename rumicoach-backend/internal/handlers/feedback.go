package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/apierror"
	"github.com/rumi/rumi-be/internal/models"
	"github.com/rumi/rumi-be/internal/services/notification"
	"github.com/rumi/rumi-be/internal/services/notification/provider/email"
	"github.com/rumi/rumi-be/internal/services/storage"
	"github.com/rumi/rumi-be/pkg/auth"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// feedbackMaxImages and feedbackMaxImageBytes bound what one report can carry. The
	// images arrive base64-encoded in the JSON body, so they pass through this process's
	// memory — the caps are what stops a single request from being a denial of service.
	feedbackMaxImages     = 3
	feedbackMaxImageBytes = 5 << 20 // 5MB per image, after decoding
	// feedbackMaxDescription is generous: people describing a bug should not be truncated.
	feedbackMaxDescription = 8000
)

// feedbackLinkTTL is how long the signed image links in the team's email stay valid. Long
// enough to survive a weekend and a triage backlog; short enough that a forwarded mail
// does not hand out an indefinite key to somebody's screenshot.
const feedbackLinkTTL = 14 * 24 * time.Hour

// feedbackImageTypes are the formats accepted. The type is sniffed from the bytes rather
// than taken from the client, so a request cannot store an executable by labelling it a
// PNG.
var feedbackImageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// SubmitFeedback implements api.ServerInterface.
//
// The row is written first and the images follow. An upload that fails costs the
// screenshot, never the report: somebody took the trouble to describe a problem, and
// losing their words because a bucket was unreachable would be the wrong trade.
func (s *Server) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok || userID == "" {
		http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req api.FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, "Invalid request body")
		return
	}

	category := string(req.Category)
	if !models.ValidFeedbackCategory(category) {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload,
			"category must be one of: bug, feedback, feature")
		return
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload,
			"description is required")
		return
	}
	if len(description) > feedbackMaxDescription {
		description = description[:feedbackMaxDescription]
	}

	images, err := decodeFeedbackImages(req.Images)
	if err != nil {
		apierror.Write(w, http.StatusBadRequest, apierror.CodeInvalidPayload, err.Error())
		return
	}

	// Headers first, body second. The client already sends X-Platform on every request, so
	// it is the more reliable of the two; the body fills in what headers cannot carry.
	feedback := models.Feedback{
		UserID:      userID,
		Category:    category,
		Description: description,
		Platform:    firstNonEmpty(headerOrNil(r, "X-Platform"), contextField(req.Context, func(c api.FeedbackContext) *string { return nil })),
		AppVersion:  headerOrNil(r, "X-App-Version"),
	}
	if req.Context != nil {
		c := *req.Context
		feedback.AppVersion = firstNonEmpty(feedback.AppVersion, c.AppVersion)
		feedback.OSVersion = c.OsVersion
		feedback.DeviceModel = c.DeviceModel
		if blob := marshalContext(c); blob != "" {
			feedback.Context = &blob
		}
	}
	if err := database.DB.Create(&feedback).Error; err != nil {
		s.logger.Error("failed to store feedback", zap.String("user_id", userID), zap.Error(err))
		http.Error(w, `{"error": "Failed to submit feedback"}`, http.StatusInternalServerError)
		return
	}

	attachments := s.storeFeedbackImages(r.Context(), &feedback, images)

	// Notifying the team is what makes the report worth collecting; a table nobody reads
	// is the same as no feature. It must not be able to fail the submission, though —
	// the user's part is done.
	go s.notifyFeedback(feedback, attachments, images)

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.FeedbackResponse{Id: &feedback.ID})
}

type decodedImage struct {
	data        []byte
	contentType string
	ext         string
}

// decodeFeedbackImages validates the payload before anything is stored: count, decoded
// size, and the real format of the bytes.
func decodeFeedbackImages(images *[]string) ([]decodedImage, error) {
	if images == nil || len(*images) == 0 {
		return nil, nil
	}
	if len(*images) > feedbackMaxImages {
		return nil, fmt.Errorf("at most %d images per report", feedbackMaxImages)
	}

	out := make([]decodedImage, 0, len(*images))
	for i, encoded := range *images {
		// Browsers and pickers commonly hand over a data URL; accept both shapes.
		if idx := strings.Index(encoded, ";base64,"); idx >= 0 {
			encoded = encoded[idx+len(";base64,"):]
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("image %d is not valid base64", i+1)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("image %d is empty", i+1)
		}
		if len(data) > feedbackMaxImageBytes {
			return nil, fmt.Errorf("image %d is %d bytes, over the %d byte limit", i+1, len(data), feedbackMaxImageBytes)
		}
		// Sniffed, never trusted: http.DetectContentType reads the magic bytes.
		contentType := http.DetectContentType(data)
		if idx := strings.Index(contentType, ";"); idx >= 0 {
			contentType = contentType[:idx]
		}
		ext, ok := feedbackImageTypes[contentType]
		if !ok {
			return nil, fmt.Errorf("image %d is %s; only PNG, JPEG, WebP and GIF are accepted", i+1, contentType)
		}
		out = append(out, decodedImage{data: data, contentType: contentType, ext: ext})
	}
	return out, nil
}

// storeFeedbackImages uploads the images and records them. Failures are logged and skipped
// rather than propagated: see SubmitFeedback.
func (s *Server) storeFeedbackImages(ctx context.Context, feedback *models.Feedback, images []decodedImage) []models.FeedbackAttachment {
	var stored []models.FeedbackAttachment
	for i, img := range images {
		path := fmt.Sprintf("%s%s/%d%s", models.FeedbackObjectPrefix(feedback.UserID), feedback.ID, i+1, img.ext)
		objectPath, err := storage.Global.Upload(ctx, path, img.contentType, img.data)
		if err != nil {
			s.logger.Error("failed to upload feedback image",
				zap.String("feedback_id", feedback.ID), zap.String("path", path), zap.Error(err))
			continue
		}
		attachment := models.FeedbackAttachment{
			FeedbackID:  feedback.ID,
			UserID:      feedback.UserID,
			ObjectPath:  objectPath,
			ContentType: img.contentType,
			SizeBytes:   len(img.data),
		}
		if err := database.DB.Create(&attachment).Error; err != nil {
			s.logger.Error("failed to record feedback attachment",
				zap.String("feedback_id", feedback.ID), zap.Error(err))
			continue
		}
		stored = append(stored, attachment)
	}
	return stored
}

// notifyFeedback emails the team. Images travel as time-limited signed links rather than
// attachments: the mail stays small whatever was uploaded, and the bytes stay in our
// bucket instead of being copied into a mailbox we do not control.
func (s *Server) notifyFeedback(feedback models.Feedback, attachments []models.FeedbackAttachment, images []decodedImage) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("panic while notifying feedback", zap.Any("panic", rec))
		}
	}()
	if notification.GlobalNotificationService == nil {
		return
	}

	links := make([]string, 0, len(attachments))
	for _, a := range attachments {
		if a.ObjectPath == "" {
			continue
		}
		url, err := storage.Global.SignedURL(context.Background(), a.ObjectPath, feedbackLinkTTL)
		if err != nil {
			s.logger.Warn("could not sign feedback image link", zap.Error(err))
			continue
		}
		links = append(links, url)
	}

	// The bytes are attached as well as linked. A link depends on the reader's client
	// agreeing to fetch it and stops working when the signature expires; an attachment does
	// neither. They are already in memory here, so this costs nothing extra.
	files := make([]email.Attachment, 0, len(images))
	for i, img := range images {
		files = append(files, email.Attachment{
			Filename: fmt.Sprintf("screenshot-%d%s", i+1, img.ext),
			MimeType: img.contentType,
			Content:  img.data,
		})
	}

	if err := notification.GlobalNotificationService.SendFeedbackNotificationEmail(
		feedback.Category, feedback.Description, contextFields(feedback), links, files,
	); err != nil {
		s.logger.Error("failed to send feedback notification", zap.Error(err))
	}
}

// marshalContext stores the long-tail diagnostics as JSON. A failure here is not worth
// failing the report over — the description is the valuable part.
func marshalContext(c api.FeedbackContext) string {
	fields := map[string]string{}
	for k, v := range map[string]*string{
		"buildNumber": c.BuildNumber,
		"locale":      c.Locale,
		"timezone":    c.Timezone,
		"screen":      c.Screen,
		"screenSize":  c.ScreenSize,
		"userAgent":   c.UserAgent,
	} {
		if v != nil && strings.TrimSpace(*v) != "" {
			fields[k] = *v
		}
	}
	if len(fields) == 0 {
		return ""
	}
	blob, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(blob)
}

// contextFields renders the diagnostics for the team's email, in a fixed order so two
// reports can be read side by side.
func contextFields(f models.Feedback) [][2]string {
	out := [][2]string{
		{"User", f.UserID},
		{"Platform", deref(f.Platform)},
		{"App version", deref(f.AppVersion)},
		{"OS", deref(f.OSVersion)},
		{"Device", deref(f.DeviceModel)},
	}
	if f.Context != nil {
		var extra map[string]string
		if json.Unmarshal([]byte(*f.Context), &extra) == nil {
			for _, k := range []string{"buildNumber", "screen", "screenSize", "locale", "timezone", "userAgent"} {
				if v, ok := extra[k]; ok {
					out = append(out, [2]string{contextLabels[k], v})
				}
			}
		}
	}
	return out
}

var contextLabels = map[string]string{
	"buildNumber": "Build",
	"screen":      "Screen",
	"screenSize":  "Viewport",
	"locale":      "Locale",
	"timezone":    "Timezone",
	"userAgent":   "User agent",
}

func firstNonEmpty(vals ...*string) *string {
	for _, v := range vals {
		if v != nil && strings.TrimSpace(*v) != "" {
			return v
		}
	}
	return nil
}

func contextField(c *api.FeedbackContext, pick func(api.FeedbackContext) *string) *string {
	if c == nil {
		return nil
	}
	return pick(*c)
}

func headerOrNil(r *http.Request, name string) *string {
	if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
		return &v
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// deleteFeedbackScope erases a user's reports and the images behind them.
//
// The bucket cannot take part in the database transaction, so the rows go first and the
// objects follow once it has committed — see the call site. Deleting by prefix rather than
// by row means an object whose row was already gone is cleaned up too, which is what makes
// the two stores converge instead of drifting apart.
func deleteFeedbackScope(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&models.FeedbackAttachment{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ?", userID).Delete(&models.Feedback{}).Error
}

// purgeFeedbackObjects removes a user's screenshots from the bucket. Call it AFTER the
// database transaction commits, never inside: an object store cannot be rolled back, so
// deleting first would destroy files for a transaction that then failed.
//
// It deletes by prefix rather than by row, which is what makes the two stores converge:
// an object whose row is already gone — a crash between the commit and this call, an older
// bug — is swept up on the next erasure instead of living in the bucket forever.
func (s *Server) purgeFeedbackObjects(ctx context.Context, userID string) {
	if !storage.Global.Enabled() {
		return
	}
	if err := storage.Global.DeletePrefix(ctx, models.FeedbackObjectPrefix(userID)); err != nil {
		// Logged, not returned: the rows are already gone and the response has been
		// decided. A leftover object is a problem to fix, not a reason to tell the user
		// their erasure failed when most of it succeeded.
		s.logger.Error("failed to delete feedback images from storage",
			zap.String("user_id", userID), zap.Error(err))
	}
}
