package auth_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/adapters/outbound/persistence"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/domain/identity"
	"github.com/Adibayuluthfiansyah/MORN1NGST4R/services/core/internal/ports/outbound"
)

// The limiter counts a user's failures whatever the source key says, so a
// second client address, were one ever possible, does not get a fresh budget.
func TestAttemptLimitFollowsTheUserAcrossSources(t *testing.T) {
	_, a, _, _ := fresh(t)
	userID := a.scalar("SELECT id FROM morningstar.users")
	db, err := persistence.Open(t.Context(), a.c.SocketDir, persistence.RoleAPI, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	attempts := db.Attempts()
	reserve := func(source, user string) bool {
		ok, err := attempts.Reserve(t.Context(), outbound.AttemptLogin, source, user, 15*time.Minute, identity.MaxFailedInWindow)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	// Account creation credited the user with one success, so the budget is
	// the limit or the limit plus one, but never unbounded.
	allowed := 0
	for i := range 2 * identity.MaxFailedInWindow {
		if reserve(fmt.Sprintf("source-%d", i), userID) {
			allowed++
		}
	}
	if allowed < identity.MaxFailedInWindow || allowed > identity.MaxFailedInWindow+1 {
		t.Fatalf("%d attempts from distinct sources were allowed for one user", allowed)
	}
	if !reserve("source-other", "") {
		t.Fatal("an unrelated source with no user was refused")
	}
}
