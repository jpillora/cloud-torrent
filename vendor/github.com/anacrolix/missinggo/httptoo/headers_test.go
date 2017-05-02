package httptoo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheControlHeaderString(t *testing.T) {
	assert.Equal(t, "public, max-age=43200", CacheControlHeader{
		MaxAge:  12 * time.Hour,
		Caching: Public,
	}.String())
}
