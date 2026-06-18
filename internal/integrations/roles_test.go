package integrations_test

import (
	"testing"

	"github.com/Kenttleton/orbiter/internal/integrations"
	"github.com/stretchr/testify/assert"
)

func TestRoleType_Export(t *testing.T) {
	assert.Equal(t, integrations.IntegrationTypeResource, integrations.RoleType("export"))
}

func TestRoleType_Multiplexer(t *testing.T) {
	assert.Equal(t, integrations.IntegrationTypeResource, integrations.RoleType("multiplexer"))
}
