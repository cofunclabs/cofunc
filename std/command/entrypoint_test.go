package command

import (
	"context"
	"testing"

	"github.com/cofunclabs/cofunc/pkg/output"
	"github.com/stretchr/testify/assert"
)

func TestCommandFunction(t *testing.T) {
	mf := New()
	assert.Equal(t, "go", mf.Driver)
	_, err := mf.EntrypointFunc(context.Background(), output.New(), "latest", map[string]string{
		"script": "echo hello cofunc && sleep 2 && echo hello cofunc2",
	})
	assert.NoError(t, err)
}
