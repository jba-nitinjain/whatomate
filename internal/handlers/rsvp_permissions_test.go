package handlers_test

import (
	"testing"

	"github.com/nikyjain/whatomate/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRSVPPermissions_InDefaults(t *testing.T) {
	perms := models.DefaultPermissions()
	found := 0
	for _, p := range perms {
		if p.Resource == models.ResourceRSVP {
			found++
		}
	}
	require.GreaterOrEqual(t, found, 4) // read, write, delete, execute (+export)
}
