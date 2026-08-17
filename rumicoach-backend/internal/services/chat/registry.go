package chat

import (
	"github.com/rumi/rumi-be/internal/services/chat/session"
	"github.com/rumi/rumi-be/internal/services/chat/session/acceptance"
	"github.com/rumi/rumi-be/internal/services/chat/session/beliefs"
	"github.com/rumi/rumi-be/internal/services/chat/session/checkin"
	"github.com/rumi/rumi-be/internal/services/chat/session/decisions"
	"github.com/rumi/rumi-be/internal/services/chat/session/energy"
	"github.com/rumi/rumi-be/internal/services/chat/session/identity"
	"github.com/rumi/rumi-be/internal/services/chat/session/movement"
	"github.com/rumi/rumi-be/internal/services/chat/session/onboarding"
	"github.com/rumi/rumi-be/internal/services/chat/session/priorities"
	"github.com/rumi/rumi-be/internal/services/chat/session/values"
	"github.com/rumi/rumi-be/internal/services/chat/session/vision"
)

// sessions is the registry of per-session implementations the chat runtime dispatches to.
// Adding a new coaching session is a matter of implementing session.Session in its own
// package under session/ and registering it here.
var sessions = session.NewRegistry(
	onboarding.New(),
	vision.New(),
	movement.New(),
	values.New(),
	energy.New(),
	decisions.New(),
	beliefs.New(),
	identity.New(),
	acceptance.New(),
	priorities.New(),
	checkin.NewDaily(),
)
