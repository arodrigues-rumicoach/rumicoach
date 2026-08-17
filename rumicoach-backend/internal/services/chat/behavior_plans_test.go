package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/rumi/rumi-be/api"
	"github.com/rumi/rumi-be/database"
	"github.com/rumi/rumi-be/internal/models"
	sess "github.com/rumi/rumi-be/internal/services/chat/session"
	"github.com/rumi/rumi-be/internal/services/chat/session/movement"
	"github.com/rumi/rumi-be/internal/services/chat/session/onboarding"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newBehaviorTestSession(t *testing.T) *ChatSession {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	// Tables are created manually to avoid the PostgreSQL "timestamp with time zone"
	// datatype scan issues on SQLite (same approach as the handlers tests).
	for _, ddl := range []string{
		`CREATE TABLE behavior_plans (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'active',
			behavior TEXT NOT NULL, identity TEXT, motive TEXT, trigger TEXT, context TEXT,
			frequency TEXT, days TEXT, obstacles TEXT, plan_b TEXT, area TEXT,
			task_id TEXT, start_date TEXT, wins_count INTEGER NOT NULL DEFAULT 0,
			last_check_in_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE behavior_check_ins (
			id TEXT PRIMARY KEY, plan_id TEXT NOT NULL, user_id TEXT NOT NULL,
			status TEXT NOT NULL, note TEXT, created_at DATETIME)`,
		`CREATE TABLE commitments (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, origin TEXT NOT NULL DEFAULT 'manual',
			title TEXT NOT NULL, type TEXT NOT NULL, days TEXT, date TEXT,
			done BOOLEAN NOT NULL DEFAULT 0, ended_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("failed to create test table: %v", err)
		}
	}
	database.DB = db
	return &ChatSession{logger: zap.NewNop(), UserID: "user-1"}
}

func TestSaveBehaviorPlanGuardsAndProjection(t *testing.T) {
	s := newBehaviorTestSession(t)

	// A vague intention without the protocol's outputs must be rejected.
	out, err := s.handleSaveBehaviorPlan(map[string]interface{}{"behavior": "walk more"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "REJECTED") || !strings.Contains(out, "identity") || !strings.Contains(out, "trigger") {
		t.Fatalf("expected rejection naming missing identity and trigger, got %q", out)
	}

	fullPlan := func(behavior string) map[string]interface{} {
		return map[string]interface{}{
			"behavior": behavior,
			"identity": "someone who protects their health",
			"trigger":  "right after my morning coffee",
			"days":     "[1,2,3,4,5]",
			"plan_b":   "5 minutes on hard days",
		}
	}

	// A complete plan saves and projects into a recurring task.
	if out, _ = s.handleSaveBehaviorPlan(fullPlan("walk 10 minutes")); !strings.Contains(out, "success") {
		t.Fatalf("expected success, got %q", out)
	}
	var plan models.BehaviorPlan
	if err := database.DB.Where("user_id = ?", s.UserID).First(&plan).Error; err != nil {
		t.Fatalf("plan not saved: %v", err)
	}
	if plan.Status != models.BehaviorPlanStatusActive || plan.TaskID == nil {
		t.Fatalf("expected active plan with projected task, got status=%s taskID=%v", plan.Status, plan.TaskID)
	}
	var task models.Commitment
	if err := database.DB.Where("id = ?", *plan.TaskID).First(&task).Error; err != nil {
		t.Fatalf("projected task not found: %v", err)
	}
	if task.Origin != models.CommitmentOriginBehavior || task.Type != "recurring" || len(task.Days) != 5 {
		t.Errorf("unexpected projected task: origin=%s type=%s days=%v", task.Origin, task.Type, task.Days)
	}

	// The active-plan cap rejects a third plan and lists the existing commitments.
	if out, _ = s.handleSaveBehaviorPlan(fullPlan("read before bed")); !strings.Contains(out, "success") {
		t.Fatalf("second plan should save, got %q", out)
	}
	out, _ = s.handleSaveBehaviorPlan(fullPlan("meditate at dawn"))
	if !strings.Contains(out, "REJECTED") || !strings.Contains(out, "walk 10 minutes") || !strings.Contains(out, "read before bed") {
		t.Fatalf("expected cap rejection listing active plans, got %q", out)
	}

	// Updating by the same behavior name changes status and removes the projection.
	out, _ = s.handleSaveBehaviorPlan(map[string]interface{}{"behavior": "walk 10 minutes", "status": "graduated"})
	if !strings.Contains(out, "graduated") {
		t.Fatalf("expected graduation message, got %q", out)
	}
	if err := database.DB.Where("id = ?", plan.ID).First(&plan).Error; err != nil {
		t.Fatalf("plan disappeared: %v", err)
	}
	if plan.Status != models.BehaviorPlanStatusGraduated || plan.TaskID != nil {
		t.Errorf("expected graduated plan without task projection, got status=%s taskID=%v", plan.Status, plan.TaskID)
	}
	var taskCount int64
	database.DB.Model(&models.Commitment{}).Where("user_id = ? AND origin = ?", s.UserID, models.CommitmentOriginBehavior).Count(&taskCount)
	if taskCount != 1 {
		t.Errorf("expected only the second plan's projected task to remain, got %d", taskCount)
	}

	// With one graduated, a new active plan fits under the cap again.
	if out, _ = s.handleSaveBehaviorPlan(fullPlan("meditate at dawn")); !strings.Contains(out, "success") {
		t.Errorf("expected success after freeing a slot, got %q", out)
	}
}

func TestLogBehaviorCheckin(t *testing.T) {
	s := newBehaviorTestSession(t)

	// No plans yet: point the model at the protocol instead.
	out, _ := s.handleLogBehaviorCheckin(map[string]interface{}{"behavior": "walk", "status": "kept"})
	if !strings.Contains(out, "no behavior plans") {
		t.Fatalf("expected no-plans guidance, got %q", out)
	}

	if out, _ = s.handleSaveBehaviorPlan(map[string]interface{}{
		"behavior": "walk 10 minutes", "identity": "someone active", "trigger": "after lunch",
	}); !strings.Contains(out, "success") {
		t.Fatalf("plan setup failed: %q", out)
	}

	// Unknown behavior lists the real plans so the model can self-correct.
	out, _ = s.handleLogBehaviorCheckin(map[string]interface{}{"behavior": "swim daily", "status": "kept"})
	if !strings.Contains(out, "walk 10 minutes") {
		t.Fatalf("expected plan listing in error, got %q", out)
	}

	// A kept check-in counts a win.
	out, _ = s.handleLogBehaviorCheckin(map[string]interface{}{"behavior": "walk 10 minutes", "status": "kept", "note": "felt great"})
	if !strings.Contains(out, "success") {
		t.Fatalf("expected success, got %q", out)
	}
	var plan models.BehaviorPlan
	database.DB.Where("user_id = ?", s.UserID).First(&plan)
	if plan.WinsCount != 1 || plan.LastCheckInAt == nil {
		t.Errorf("expected wins=1 and last check-in set, got wins=%d lastCheckIn=%v", plan.WinsCount, plan.LastCheckInAt)
	}

	// A miss stores information but never subtracts wins, and the response forbids judging.
	out, _ = s.handleLogBehaviorCheckin(map[string]interface{}{"behavior": "walk 10 minutes", "status": "missed", "note": "work ran late"})
	if !strings.Contains(out, "information") || !strings.Contains(out, "Never judge") {
		t.Fatalf("missed response must frame the miss as information, got %q", out)
	}
	database.DB.Where("user_id = ?", s.UserID).First(&plan)
	if plan.WinsCount != 1 {
		t.Errorf("wins must never decrease, got %d", plan.WinsCount)
	}
	var checkinCount int64
	database.DB.Model(&models.BehaviorCheckIn{}).Where("plan_id = ?", plan.ID).Count(&checkinCount)
	if checkinCount != 2 {
		t.Errorf("expected 2 check-ins recorded, got %d", checkinCount)
	}
}

func TestBehaviorProtocolWiring(t *testing.T) {
	// Non-onboarding sessions carry the protocol and its tools; onboarding must not.
	mv := movement.New()
	if !strings.Contains(mv.Instructions(models.StateMovement, sess.Context{}), "BEHAVIOR CHANGE PROTOCOL") {
		t.Error("movement instructions must include the behavior change protocol")
	}
	found := false
	for _, name := range mv.ToolNames(models.StateMovement) {
		if name == "save_behavior_plan" {
			found = true
		}
	}
	if !found {
		t.Error("movement must declare save_behavior_plan")
	}

	for _, state := range []models.SessionState{models.StateOnboardingIntro, models.StateVisionIdealLife, models.StateVisionWheelOfLife} {
		for _, name := range onboarding.ToolNames(state) {
			if name == "save_behavior_plan" || name == "log_behavior_checkin" {
				t.Errorf("onboarding state %s must NOT declare behavior tools", state)
			}
		}
	}

	// The context block renders active plans with the follow-up directive and keeps
	// parked ones quiet.
	area := "Health"
	plans := []models.BehaviorPlan{
		{ID: "p1", Behavior: "walk 10 minutes", Identity: "someone active", Trigger: "after lunch", Status: models.BehaviorPlanStatusActive, WinsCount: 3, Area: &area},
		{ID: "p2", Behavior: "meditate at dawn", Status: models.BehaviorPlanStatusParked},
	}
	block := FormatBehaviorPlansContext(plans, map[string]models.BehaviorCheckIn{})
	if !strings.Contains(block, "ACTIVE BEHAVIOR COMMITMENTS") || !strings.Contains(block, "walk 10 minutes") ||
		!strings.Contains(block, "wins so far: 3") || !strings.Contains(block, "do NOT bring these up") {
		t.Errorf("unexpected behavior plans block: %q", block)
	}
	if FormatBehaviorPlansContext(nil, nil) != "" {
		t.Error("no plans must render an empty block")
	}
}

// The end-of-session synthesis screen (Filipa's "ecrã de síntese") is built from data the
// session already stored, in the user's language, with next-session questions as
// localization keys.
func TestBuildSessionSummary(t *testing.T) {
	s := newBehaviorTestSession(t)
	for _, ddl := range []string{
		`CREATE TABLE wheel_of_life_exercises (id TEXT PRIMARY KEY, user_id TEXT, session_id TEXT, data TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE user_memories (id TEXT PRIMARY KEY, user_id TEXT, category TEXT, content TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE identity_reflections (id TEXT PRIMARY KEY, user_id TEXT, session_id TEXT, learned_identity TEXT, what_it_gave TEXT, what_it_costs TEXT, who_becoming TEXT, qualities TEXT, evidence TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE acceptance_reflections (id TEXT PRIMARY KEY, user_id TEXT, session_id TEXT, expected TEXT, reality TEXT, cannot_control TEXT, can_influence TEXT, choose_to_accept TEXT, where_i_act TEXT, next_step TEXT, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := database.DB.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}

	vision := "Uma vida na Tailândia com o André"
	focusArea := "Propósito"
	s.User = &models.User{IdealLifeVision: &vision, FocusArea: &focusArea}
	s.SessionDB.ID = "sess-1"
	s.SessionDB.StartTime = time.Now().Add(-10 * time.Minute)

	now := time.Now()
	database.DB.Exec(`INSERT INTO wheel_of_life_exercises (id, user_id, data, created_at) VALUES ('w1', 'user-1', '[{"name":"Propósito","currentScore":4,"reasoning":"Trabalho só para pagar contas"}]', ?)`, now)
	database.DB.Exec(`INSERT INTO user_memories (id, user_id, category, content, created_at) VALUES ('m1', 'user-1', 'insight', 'É mais urgente do que eu pensava', ?)`, now)

	// Vision summary: vision + priority area with score + insight + movement seed.
	// (The onboarding intro has no synthesis screen — it produces none of these anchors.)
	s.SessionType = api.SessionTypeSessionVision
	data := s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("vision summary must be built")
	}
	if data["vision"] != vision || data["key_insight"] != "É mais urgente do que eu pensava" {
		t.Errorf("unexpected vision/insight: %v / %v", data["vision"], data["key_insight"])
	}
	area, _ := data["priority_area"].(map[string]interface{})
	if area == nil || area["name"] != "Propósito" || area["score"] != 4 {
		t.Errorf("unexpected priority area: %v", area)
	}
	next, _ := data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_movement_blockers" {
		t.Errorf("unexpected next session: %v", data["next_session"])
	}

	// The early stage (start of the synthesis) omits next_session — added later only once
	// the goodbye actually reaches the bridge sentence.
	early := s.buildSessionSummary(false)
	if early == nil {
		t.Fatal("early-stage vision summary must be built")
	}
	if _, ok := early["next_session"]; ok {
		t.Errorf("early-stage summary must not include next_session: %v", early["next_session"])
	}
	if early["vision"] != vision {
		t.Errorf("early-stage summary missing vision: %v", early["vision"])
	}

	// Movement summary adds the registered commitments and the values seed.
	s.SessionType = api.SessionTypeSessionMovement
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, date, done, created_at) VALUES ('t1', 'user-1', 'manual', 'Marcar consulta', 'one_time', '2026-07-19', 0, ?)`, now)
	data = s.buildSessionSummary(true)
	commitments, _ := data["commitments"].([]map[string]interface{})
	if len(commitments) != 1 || commitments[0]["title"] != "Marcar consulta" {
		t.Errorf("unexpected commitments: %v", data["commitments"])
	}
	next, _ = data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_values_why" {
		t.Errorf("unexpected next session: %v", data["next_session"])
	}

	// Values summary: it gets a card too (QA: it used to end with none), carrying the
	// CHOSEN top values from users.top_values (never the free-text values memories —
	// QA saw three long memory sentences on the card) and the energy bridge.
	s.SessionType = api.SessionTypeSessionValues
	s.User.TopValues = models.StringSlice{"Crescimento", "Amor", "Família"}
	data = s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("values summary must be built")
	}
	values, _ := data["values"].([]string)
	if len(values) != 3 || values[0] != "Crescimento" {
		t.Errorf("unexpected values on the card: %v", data["values"])
	}
	next, _ = data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_energy_capacity" {
		t.Errorf("unexpected next session: %v", data["next_session"])
	}

	// Beliefs bridges to Identity, Identity to Acceptance, Acceptance to Priorities;
	// Priorities closes the first journey pass — card, but no fixed next-session bridge
	// (the journey cycles from there).
	s.SessionType = api.SessionTypeSessionBeliefs
	data = s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("beliefs summary must be built")
	}
	next, _ = data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_identity_becoming" {
		t.Errorf("unexpected beliefs next session: %v", data["next_session"])
	}
	s.SessionType = api.SessionTypeSessionPriorities
	data = s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("priorities summary must be built")
	}
	if _, ok := data["next_session"]; ok {
		t.Errorf("priorities summary must not include next_session: %v", data["next_session"])
	}

	// The Acceptance card carries the structured reflection captured by
	// save_acceptance_reflection, replaced (never stacked) on a correcting re-call,
	// and bridges to Priorities.
	s.SessionType = api.SessionTypeSessionAcceptance
	if _, err := s.handleSaveAcceptanceReflection(map[string]interface{}{
		"expected":         "Que o meu parceiro percebesse o que eu precisava sem eu ter de pedir",
		"reality":          "Comunicamos de formas diferentes",
		"cannot_control":   "A forma como ele interpreta o que digo",
		"can_influence":    "A clareza com que comunico",
		"choose_to_accept": "Não controlo a resposta do outro",
		"where_i_act":      "Dizer claramente o que preciso",
		"next_step":        "Ter a conversa que tenho adiado",
	}); err != nil {
		t.Fatalf("handleSaveAcceptanceReflection failed: %v", err)
	}
	if _, err := s.handleSaveAcceptanceReflection(map[string]interface{}{
		"expected":         "Que o meu parceiro percebesse o que eu precisava sem eu ter de pedir",
		"reality":          "Comunicamos de formas diferentes e nem tudo foi dito",
		"cannot_control":   "A forma como ele interpreta o que digo",
		"can_influence":    "A clareza com que comunico",
		"choose_to_accept": "Não controlo a resposta do outro",
		"where_i_act":      "Dizer claramente o que preciso",
	}); err != nil {
		t.Fatalf("second handleSaveAcceptanceReflection failed: %v", err)
	}
	var acceptReflections []models.AcceptanceReflection
	if err := database.DB.Where("user_id = ?", "user-1").Find(&acceptReflections).Error; err != nil || len(acceptReflections) != 1 {
		t.Fatalf("expected exactly one acceptance reflection after a correcting re-call, got %d (err=%v)", len(acceptReflections), err)
	}
	data = s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("acceptance summary must be built")
	}
	aref, _ := data["acceptance_reflection"].(map[string]interface{})
	if aref == nil || aref["reality"] != "Comunicamos de formas diferentes e nem tudo foi dito" {
		t.Errorf("unexpected acceptance reflection on the card: %v", data["acceptance_reflection"])
	}
	if _, ok := aref["next_step"]; ok {
		t.Errorf("omitted next_step must not appear on the card: %v", aref["next_step"])
	}
	next, _ = data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_priorities_attention" {
		t.Errorf("unexpected acceptance next session: %v", data["next_session"])
	}

	// The Identity card carries the structured reflection captured by
	// save_identity_reflection, replaced (never stacked) on a correcting re-call.
	s.SessionType = api.SessionTypeSessionIdentity
	if _, err := s.handleSaveIdentityReflection(map[string]interface{}{
		"learned_identity": "Alguém que resolve tudo sozinha",
		"what_it_gave":     "Independência e resiliência",
		"what_it_costs":    "Dificuldade em pedir ajuda",
		"who_becoming":     "Independente, mas segura o suficiente para deixar os outros entrar",
		"qualities":        []interface{}{"Abertura", "Coragem"},
		"evidence":         "Pedir ajuda uma vez esta semana",
	}); err != nil {
		t.Fatalf("handleSaveIdentityReflection failed: %v", err)
	}
	if _, err := s.handleSaveIdentityReflection(map[string]interface{}{
		"learned_identity": "Alguém que resolve tudo sozinha",
		"what_it_gave":     "Independência",
		"what_it_costs":    "Dificuldade em pedir ajuda",
		"who_becoming":     "Independente, mas aberta ao apoio dos outros",
		"qualities":        []interface{}{"Abertura", "Confiança"},
	}); err != nil {
		t.Fatalf("second handleSaveIdentityReflection failed: %v", err)
	}
	var reflections []models.IdentityReflection
	if err := database.DB.Where("user_id = ?", "user-1").Find(&reflections).Error; err != nil || len(reflections) != 1 {
		t.Fatalf("expected exactly one reflection after a correcting re-call, got %d (err=%v)", len(reflections), err)
	}
	data = s.buildSessionSummary(true)
	if data == nil {
		t.Fatal("identity summary must be built")
	}
	ref, _ := data["identity_reflection"].(map[string]interface{})
	if ref == nil || ref["who_becoming"] != "Independente, mas aberta ao apoio dos outros" {
		t.Errorf("unexpected identity reflection on the card: %v", data["identity_reflection"])
	}
	if _, ok := ref["evidence"]; ok {
		t.Errorf("omitted evidence must not appear on the card: %v", ref["evidence"])
	}
	next, _ = data["next_session"].(map[string]string)
	if next["question_key"] != "next_question_acceptance_expectations" {
		t.Errorf("unexpected identity next session: %v", data["next_session"])
	}

	// The same intention saved twice (two tools, or a model retry — QA saw the commitment
	// doubled on the card) renders exactly once, matched on normalized title.
	s.SessionType = api.SessionTypeSessionMovement
	database.DB.Exec(`INSERT INTO commitments (id, user_id, origin, title, type, date, done, created_at) VALUES ('t2', 'user-1', 'plan', 'marcar consulta.', 'one_time', '2026-07-19', 0, ?)`, now)
	data = s.buildSessionSummary(true)
	commitments, _ = data["commitments"].([]map[string]interface{})
	if len(commitments) != 1 || commitments[0]["title"] != "Marcar consulta" {
		t.Errorf("duplicate commitment must render once on the card: %v", data["commitments"])
	}

	// Sessions without a designed synthesis screen yield nothing.
	s.SessionType = api.SessionTypeCheckin
	if s.buildSessionSummary(true) != nil {
		t.Error("checkin must not build a summary")
	}
}

func TestHandleSaveVisionCommitment(t *testing.T) {
	s := newBehaviorTestSession(t)
	for _, ddl := range []string{
		`CREATE TABLE wheel_of_life_exercises (id TEXT PRIMARY KEY, user_id TEXT, session_id TEXT, data TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE user_memories (id TEXT PRIMARY KEY, user_id TEXT, category TEXT, content TEXT, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := database.DB.Exec(ddl).Error; err != nil {
			t.Fatalf("ddl failed: %v", err)
		}
	}
	s.Location = time.UTC
	s.SessionType = api.SessionTypeSessionVision
	s.SessionDB.ID = "sess-vision-1"
	s.SessionDB.StartTime = time.Now().Add(-5 * time.Minute)
	vision := "Uma vida na Tailândia com o André"
	s.User = &models.User{IdealLifeVision: &vision}

	if _, err := s.handleSaveVisionCommitment(map[string]interface{}{"commitment": ""}); err == nil {
		t.Error("expected an error for an empty commitment")
	}

	out, err := s.handleSaveVisionCommitment(map[string]interface{}{"commitment": "Vou refletir sobre a minha carreira."})
	if err != nil {
		t.Fatalf("handleSaveVisionCommitment failed: %v", err)
	}
	if !strings.Contains(out, "success") {
		t.Errorf("unexpected output: %s", out)
	}

	var saved models.Commitment
	if err := database.DB.Where("user_id = ?", "user-1").First(&saved).Error; err != nil {
		t.Fatalf("commitment was not persisted: %v", err)
	}
	if saved.Origin != models.CommitmentOriginPlan {
		t.Errorf("expected origin 'plan', got %q", saved.Origin)
	}
	if saved.Type != "one_time" {
		t.Errorf("expected type 'one_time', got %q", saved.Type)
	}
	if saved.Title != "Vou refletir sobre a minha carreira." {
		t.Errorf("unexpected title: %q", saved.Title)
	}
	wantDate := time.Now().In(time.UTC).Format("2006-01-02")
	if saved.Date == nil || *saved.Date != wantDate {
		t.Errorf("expected date %q, got %v", wantDate, saved.Date)
	}

	// The same commitment must surface in the Vision session_summary payload — this is the
	// synthesis screen's only view into what the user just committed to.
	data := s.buildSessionSummary(true)
	commitments, _ := data["commitments"].([]map[string]interface{})
	if len(commitments) != 1 || commitments[0]["title"] != "Vou refletir sobre a minha carreira." {
		t.Errorf("expected the vision commitment in the summary, got: %v", data["commitments"])
	}

	// A second call (the model correcting a premature/fabricated first capture, QA) must
	// REPLACE the prior one, not stack a duplicate.
	if _, err := s.handleSaveVisionCommitment(map[string]interface{}{"commitment": "Vou candidatar-me a uma vaga nova."}); err != nil {
		t.Fatalf("second handleSaveVisionCommitment failed: %v", err)
	}
	var all []models.Commitment
	if err := database.DB.Where("user_id = ?", "user-1").Find(&all).Error; err != nil {
		t.Fatalf("failed to list commitments: %v", err)
	}
	if len(all) != 1 || all[0].Title != "Vou candidatar-me a uma vaga nova." {
		t.Errorf("expected the second call to replace the first, got: %v", all)
	}

	// An ongoing intention ("go to bed by 22h30") saved with recurring=true becomes a
	// daily habit with a two-week horizon, not a one-off for today (QA).
	if _, err := s.handleSaveVisionCommitment(map[string]interface{}{"commitment": "Deitar-me entre as 22h15 e as 22h30", "recurring": true}); err != nil {
		t.Fatalf("recurring handleSaveVisionCommitment failed: %v", err)
	}
	if err := database.DB.Where("user_id = ?", "user-1").Find(&all).Error; err != nil {
		t.Fatalf("failed to list commitments: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("recurring call must still replace, got %d rows", len(all))
	}
	rec := all[0]
	if rec.Type != "recurring" || len(rec.Days) != 7 || rec.Date != nil || rec.EndedAt == nil {
		t.Errorf("unexpected recurring commitment shape: type=%q days=%v date=%v endedAt=%v", rec.Type, rec.Days, rec.Date, rec.EndedAt)
	}
}
