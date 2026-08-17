package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Feedback categories, matching the three tabs on the app's Send Feedback screen.
const (
	FeedbackCategoryBug     = "bug"
	FeedbackCategoryGeneral = "feedback"
	FeedbackCategoryFeature = "feature"
)

// ValidFeedbackCategory reports whether c is one of the three the app can send.
func ValidFeedbackCategory(c string) bool {
	switch c {
	case FeedbackCategoryBug, FeedbackCategoryGeneral, FeedbackCategoryFeature:
		return true
	}
	return false
}

// Feedback is one report a user sent from the app: a bug, a remark, or a request.
type Feedback struct {
	ID       string `gorm:"primaryKey;type:text"`
	UserID   string `gorm:"index;not null;type:text"`
	Category string `gorm:"not null;type:text"`
	// Description is the user's own words, the whole point of the record.
	Description string `gorm:"not null;type:text"`
	// Platform, AppVersion, OSVersion and DeviceModel are columns rather than part of the
	// blob below because they are what you group by: "how many crash reports on 1.4.2?",
	// "is this only on Android?". A bug report without them costs a round trip to the user
	// before anyone can act on it, which in practice means the report is lost.
	Platform    *string `gorm:"index;type:text"`
	AppVersion  *string `gorm:"index;type:text"`
	OSVersion   *string `gorm:"type:text"`
	DeviceModel *string `gorm:"type:text"`
	// Context is the long tail of client diagnostics as JSON — locale, timezone, the screen
	// they were on, the viewport, the user agent. A column each would be a migration every
	// time a platform learns to report one more thing, and none of it is queried.
	Context   *string   `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime;index;type:timestamp with time zone"`
}

func (f *Feedback) BeforeCreate(tx *gorm.DB) (err error) {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return
}

// FeedbackAttachment points at an image in the object store. The bytes never live in the
// database; this row is the index into the bucket.
//
// UserID is denormalized from Feedback on purpose: the erasure paths need to find every
// object belonging to a user, and they should not have to join to do it.
type FeedbackAttachment struct {
	ID         string `gorm:"primaryKey;type:text"`
	FeedbackID string `gorm:"index;not null;type:text"`
	UserID     string `gorm:"index;not null;type:text"`
	// ObjectPath is the key inside the deployment's bucket. Empty when the deployment has
	// no bucket configured and the upload was discarded.
	ObjectPath  string    `gorm:"type:text"`
	ContentType string    `gorm:"type:text"`
	SizeBytes   int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index;type:timestamp with time zone"`
}

func (a *FeedbackAttachment) BeforeCreate(tx *gorm.DB) (err error) {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return
}

// FeedbackObjectPrefix is where a user's feedback images live in the bucket. Everything is
// under one user-scoped prefix so erasing an account is a single prefix delete rather than
// a walk over rows that may already be gone.
func FeedbackObjectPrefix(userID string) string {
	return "feedback/" + userID + "/"
}
