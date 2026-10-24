package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"golang.org/x/time/rate"
	"strings"
)

// A fixed 64 KiB burst admits peer request blocks and HTTP reads without
// allowing a large configured rate to create a correspondingly huge burst.
const bandwidthBurst = 64 << 10
const maxBandwidthKiB = 1 << 30

func validateBandwidth(c Config) error {
	if c.DownloadLimitKiB < 0 || c.UploadLimitKiB < 0 || c.DownloadLimitKiB > maxBandwidthKiB || c.UploadLimitKiB > maxBandwidthKiB {
		return errors.New("bandwidth limits must be whole KiB/s from 0 (unlimited) to 1073741824")
	}
	return nil
}
func bandwidthRate(kib int64) rate.Limit {
	if kib == 0 {
		return rate.Inf
	}
	return rate.Limit(kib * 1024)
}
func (e *Engine) applyBandwidthLocked(c Config) {
	e.downloadLimiter.SetLimit(bandwidthRate(c.DownloadLimitKiB))
	e.uploadLimiter.SetLimit(bandwidthRate(c.UploadLimitKiB))
}

// encoding/json normally treats null numeric fields as zero. That must not
// silently disable an existing bandwidth cap.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("configuration must be an object")
	}
	for key, value := range fields {
		if (strings.EqualFold(key, "DownloadLimitKiB") || strings.EqualFold(key, "UploadLimitKiB")) && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("bandwidth limit cannot be null")
		}
	}
	next := plain(*c)
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	if err := validateBandwidth(Config(next)); err != nil {
		return err
	}
	*c = Config(next)
	return nil
}
