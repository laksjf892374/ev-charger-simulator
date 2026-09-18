package identifier_test

import (
	"testing"

	"cposim/gateway/identifier"
	"cposim/internal/assert"
)

func TestNewID(t *testing.T) {
	t.Run("issues increasing IDs independently per prefix", func(t *testing.T) {
		// Given
		identifierGateway := identifier.NewSequentialGateway()

		// When
		firstSessionID := identifierGateway.NewID("SES")
		secondSessionID := identifierGateway.NewID("SES")
		firstCDRID := identifierGateway.NewID("CDR")

		// Then
		assert.Equal(t, firstSessionID, "SES-000001")
		assert.Equal(t, secondSessionID, "SES-000002")
		assert.Equal(t, firstCDRID, "CDR-000001")
	})
}
