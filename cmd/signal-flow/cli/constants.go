package cli

import "github.com/google/uuid"

// devTenantID is the deterministic dev user UUID seeded by migration 000005.
// Used for single-user CLI mode.
var devTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
