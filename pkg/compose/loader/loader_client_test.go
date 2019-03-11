package loader

import (
	"testing"

	"gotest.tools/assert"
)

func TestComposeWithEnv(t *testing.T) {
	input, err := LoadComposefile([]string{"../tests/fixtures/default-env-file/docker-compose.yml"})
	assert.NilError(t, err)
	_, err = ParseComposeInput(*input)
	assert.ErrorContains(t, err, "IMAGE")
}
