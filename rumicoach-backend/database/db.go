package database

import (
	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

// AuthDB is the dedicated identity database (identities, verification codes),
// connected only on the auth plane. Data planes never hold identity data.
var AuthDB *gorm.DB
var dbLogger *zap.Logger

// Auth returns the identity database, falling back to the regional DB in local
// single-database development setups.
func Auth() *gorm.DB {
	if AuthDB != nil {
		return AuthDB
	}
	return DB
}

// ConnectAuthDB connects the identity database and migrates its schema. Call only
// on the auth plane. With no AUTH_DATABASE_URL configured (local development) the
// identity tables share the regional database.
func ConnectAuthDB(l *zap.Logger) {
	url := config.AppConfig.AuthDatabaseURL
	if url == "" || url == config.AppConfig.DatabaseURL {
		l.Info("No separate AUTH_DATABASE_URL; identity tables share the regional database")
		AuthDB = DB
	} else {
		gormLogger := gormlogger.Default.LogMode(gormlogger.Info)
		if config.AppConfig.LogLevel == "INFO" || config.AppConfig.LogLevel == "ERROR" {
			gormLogger = gormlogger.Default.LogMode(gormlogger.Error)
		}
		var err error
		AuthDB, err = gorm.Open(postgres.Open(url), &gorm.Config{Logger: gormLogger})
		if err != nil {
			l.Error("Failed to connect to auth database!", zap.Error(err))
			panic(err)
		}
		l.Info("Connected Successfully to Auth Database")
	}

	if err := AuthDB.AutoMigrate(&models.Identity{}, &models.VerificationCode{}); err != nil {
		l.Error("Failed to migrate auth database", zap.Error(err))
		panic(err)
	}

	// Case-insensitive email uniqueness: the column's own unique constraint is
	// case-sensitive, which let "Mando@x.com" and "mando@x.com" coexist as two
	// accounts. Handlers now normalize to lowercase, and this index is the hard
	// guarantee. Best-effort: creation fails while pre-existing case-duplicates
	// remain (they must be merged by hand), and boot must not hinge on that.
	if err := AuthDB.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_identities_email_lower ON identities (LOWER(email))`,
	).Error; err != nil {
		l.Warn("Could not create case-insensitive email unique index (duplicate emails still in table?)", zap.Error(err))
	}
}

func ConnectDB(l *zap.Logger) {
	dbLogger = l
	var err error

	gormLogger := gormlogger.Default.LogMode(gormlogger.Info)
	if config.AppConfig.LogLevel == "INFO" || config.AppConfig.LogLevel == "ERROR" {
		gormLogger = gormlogger.Default.LogMode(gormlogger.Error)
	}

	DB, err = gorm.Open(postgres.Open(config.AppConfig.DatabaseURL), &gorm.Config{
		Logger: gormLogger,
	})

	if err != nil {
		dbLogger.Error("Failed to connect to database!", zap.Error(err))
		panic(err)
	}

	dbLogger.Info("Connected Successfully to Database")

	// Run Migrations
	Migrate()
}

func Migrate() {
	dbLogger.Info("Running Auto-migrations...")

	// Pre-migration: Rename column commitment_plan_data to tasks if it exists
	if DB.Migrator().HasColumn(&models.User{}, "commitment_plan_data") {
		dbLogger.Info("Renaming column commitment_plan_data to tasks in users table...")
		if err := DB.Migrator().RenameColumn(&models.User{}, "commitment_plan_data", "tasks"); err != nil {
			dbLogger.Error("Failed to rename column commitment_plan_data to tasks in users table", zap.Error(err))
		}
	}

	// Pre-migration: tasks became commitments (goals were removed; an commitment stands on its
	// own under users.focus_area). Rename the table, the DailyJourney JSONB column, and
	// fold the legacy 'goal' origin into 'plan' so existing rows survive the deploy.
	if DB.Migrator().HasTable("tasks") && !DB.Migrator().HasTable("commitments") {
		dbLogger.Info("Renaming table tasks to commitments...")
		if err := DB.Migrator().RenameTable("tasks", "commitments"); err != nil {
			dbLogger.Error("Failed to rename table tasks to commitments", zap.Error(err))
		}
	}
	if DB.Migrator().HasTable("daily_growths") && DB.Migrator().HasColumn(&models.DailyJourney{}, "tasks") {
		dbLogger.Info("Renaming column tasks to commitments in daily_growths table...")
		if err := DB.Migrator().RenameColumn(&models.DailyJourney{}, "tasks", "commitments"); err != nil {
			dbLogger.Error("Failed to rename column tasks to commitments in daily_growths table", zap.Error(err))
		}
	}
	if DB.Migrator().HasTable("commitments") {
		if res := DB.Exec("UPDATE commitments SET origin = 'plan' WHERE origin = 'goal'"); res.Error != nil {
			dbLogger.Error("Failed to fold commitment origin goal into plan", zap.Error(res.Error))
		} else if res.RowsAffected > 0 {
			dbLogger.Info("Folded legacy commitment origin goal into plan", zap.Int64("count", res.RowsAffected))
		}
	}

	// Pre-migration: the Journey screen became the Journey screen, and the rename went all
	// the way down rather than stopping at the label — a product that calls something two
	// different names in two places is a product people describe wrongly to each other.
	//
	// These run BEFORE AutoMigrate: the models now resolve to the new names, so without the
	// renames AutoMigrate would create empty tables and columns alongside the populated old
	// ones and every user would look brand new.
	//
	// Note what is NOT renamed: the VALUES in journey_quote_category. "growth" there is one
	// of the six quote themes — learning, progress, becoming better over time — and has
	// nothing to do with the screen. Renaming it would be both wrong and destructive.
	if DB.Migrator().HasTable("daily_growths") && !DB.Migrator().HasTable("daily_journeys") {
		dbLogger.Info("Renaming table daily_growths to daily_journeys...")
		if err := DB.Migrator().RenameTable("daily_growths", "daily_journeys"); err != nil {
			dbLogger.Error("Failed to rename table daily_growths to daily_journeys", zap.Error(err))
		}
	}
	if DB.Migrator().HasColumn(&models.User{}, "growth_theme") {
		dbLogger.Info("Renaming column growth_theme to journey_theme in users table...")
		if err := DB.Migrator().RenameColumn(&models.User{}, "growth_theme", "journey_theme"); err != nil {
			dbLogger.Error("Failed to rename column growth_theme to journey_theme", zap.Error(err))
		}
	}
	if DB.Migrator().HasColumn(&models.User{}, "growth_quote_category") {
		dbLogger.Info("Renaming column growth_quote_category to journey_quote_category in users table...")
		if err := DB.Migrator().RenameColumn(&models.User{}, "growth_quote_category", "journey_quote_category"); err != nil {
			dbLogger.Error("Failed to rename column growth_quote_category to journey_quote_category", zap.Error(err))
		}
	}

	// Pre-migration: push_notifications became the channel-agnostic notifications
	// table; rename it so scheduled-but-unsent rows survive the deploy.
	if DB.Migrator().HasTable("push_notifications") && !DB.Migrator().HasTable("notifications") {
		dbLogger.Info("Renaming table push_notifications to notifications...")
		if err := DB.Migrator().RenameTable("push_notifications", "notifications"); err != nil {
			dbLogger.Error("Failed to rename table push_notifications to notifications", zap.Error(err))
		}
	}

	// Pre-migration: ChannelBinding was renamed to Integration.
	if DB.Migrator().HasTable("channel_bindings") && !DB.Migrator().HasTable("integrations") {
		dbLogger.Info("Renaming table channel_bindings to integrations...")
		if err := DB.Migrator().RenameTable("channel_bindings", "integrations"); err != nil {
			dbLogger.Error("Failed to rename table channel_bindings to integrations", zap.Error(err))
		}
	}

	if DB.Migrator().HasTable("actions") && !DB.Migrator().HasTable("commitments") {
		dbLogger.Info("Renaming table actions to commitments...")
		if err := DB.Migrator().RenameTable("actions", "commitments"); err != nil {
			dbLogger.Error("Failed to rename table actions to commitments", zap.Error(err))
		}
	}

	err := DB.AutoMigrate(
		&models.User{},
		&models.CommunicationSession{},
		&models.Feedback{},
		&models.FeedbackAttachment{},
		&models.UserMemory{},
		&models.EisenhowerMatrixExercise{},
		&models.UserAppOpen{},
		&models.Commitment{},
		&models.WheelOfLifeExercise{},
		&models.DailyJourney{},
		&models.Recommendation{},
		&models.Notification{},
		&models.UserDevice{},
		&models.Lead{},
		&models.BalanceTransaction{},
		&models.Integration{},
		&models.ChannelMessage{},
		&models.AIUsageRecord{},
		&models.ChannelFollowUp{},
		&models.BehaviorPlan{},
		&models.TwilioLog{},
		&models.BehaviorCheckIn{},
		&models.UserBadge{},
		&models.CommitmentCompletion{},
		&models.IdentityReflection{},
		&models.AcceptanceReflection{},
	)

	if err != nil {
		dbLogger.Error("Failed to migrate database", zap.Error(err))
		panic(err)
	}

	backfillLegacyUserStates()

	dbLogger.Info("Auto-migrations completed.")
}

// backfillLegacyUserStates normalizes pre-rename users.state values onto the current
// state names: 'ONBOARDING' (the old column default — every freshly provisioned user
// carried it and was mistakenly routed to the daily check-in prompt) and
// 'ONBOARDING_MEMORIES' fold into ONBOARDING_INTRO; 'WHEEL_OF_LIFE_CATEGORIES' and
// 'WHEEL_OF_LIFE' fold into ONBOARDING_WHEEL_OF_LIFE. It also pins the column default,
// which AutoMigrate does not reliably update on existing columns. Idempotent: the
// UPDATEs match nothing once normalized.
func backfillLegacyUserStates() {
	// Fresh accounts rest at VISION_IDEAL_LIFE: the app starts the onboarding intro
	// explicitly after account creation, so the column default is what follows it.
	if err := DB.Exec(`ALTER TABLE users ALTER COLUMN state SET DEFAULT 'VISION_IDEAL_LIFE'`).Error; err != nil {
		dbLogger.Error("Failed to update users.state column default", zap.Error(err))
	}

	legacy := map[string][]string{
		string(models.StateOnboardingIntro): {string(models.StateLegacyOnboarding), "ONBOARDING_MEMORIES"},
		// Splitting onboarding into intro + Vision moved these five states to the Vision
		// session and renamed them off the ONBOARDING_ prefix (IsOnboarding keys off it).
		// A user mid-flow at deploy time keeps their exact position in the script.
		string(models.StateVisionIdealLife):        {"ONBOARDING_IDEAL_LIFE_VISION"},
		string(models.StateVisionWheelOfLife):      {"WHEEL_OF_LIFE_CATEGORIES", "WHEEL_OF_LIFE", "ONBOARDING_WHEEL_OF_LIFE"},
		string(models.StateVisionMetaphor):         {"ONBOARDING_METAPHOR"},
		string(models.StateVisionEmotionalClosing): {"ONBOARDING_EMOTIONAL_CLOSING"},
		string(models.StateVisionEndingSession):    {"ONBOARDING_ENDING_SESSION"},
	}
	for target, old := range legacy {
		res := DB.Model(&models.User{}).Where("state IN ?", old).Update("state", target)
		if res.Error != nil {
			dbLogger.Error("Failed to normalize legacy user states", zap.String("target", target), zap.Error(res.Error))
		} else if res.RowsAffected > 0 {
			dbLogger.Info("Normalized legacy user states", zap.String("target", target), zap.Int64("count", res.RowsAffected))
		}
	}
}
