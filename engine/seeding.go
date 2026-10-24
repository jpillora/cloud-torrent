package engine

import (
	"errors"
	"math"
	"time"
)

const maxSafeTransferBytes = int64(1<<53 - 1)

type seedAccount struct {
	Uploaded   int64
	SeedNanos  int64
	Override   bool
	StopReason string
}

func validateSeeding(c Config) error {
	if math.IsNaN(c.SeedRatioLimit) || math.IsInf(c.SeedRatioLimit, 0) || c.SeedRatioLimit < 0 || c.SeedRatioLimit > 1000 {
		return errors.New("seed ratio must be 0 (disabled) to 1000")
	}
	if c.SeedTimeMinutes < 0 || c.SeedTimeMinutes > 525600 {
		return errors.New("seed time must be whole minutes from 0 (disabled) to 525600")
	}
	return nil
}
func (e *Engine) nowLocked() time.Time {
	if e.clock != nil {
		return e.clock()
	}
	return time.Now()
}
func (e *Engine) accountSeedingLocked(now time.Time) {
	for _, model := range e.ts {
		if model.t != nil {
			stats := model.t.Stats()
			uploaded := stats.BytesWrittenData.Int64()
			if uploaded > model.lastUploaded {
				delta := uploaded - model.lastUploaded
				if delta > maxSafeTransferBytes-model.account.Uploaded {
					model.account.Uploaded = maxSafeTransferBytes
				} else {
					model.account.Uploaded += delta
				}
				e.accountDirty = true
			}
			model.lastUploaded = uploaded
		}
		if model.wasSeeding && !model.accountedAt.IsZero() && now.After(model.accountedAt) {
			delta := now.Sub(model.accountedAt).Nanoseconds()
			if delta > math.MaxInt64-model.account.SeedNanos {
				model.account.SeedNanos = math.MaxInt64
			} else {
				model.account.SeedNanos += delta
			}
			e.accountDirty = true
		}
		model.accountedAt = now
	}
}
func seedPolicyReason(reason string) bool { return reason == "seed-ratio" || reason == "seed-time" }
func (e *Engine) seedLimitLocked(model *Torrent, complete bool) string {
	if model.account.Override || !e.config.EnableUpload || !e.config.EnableSeeding {
		return ""
	}
	if complete && e.config.SeedRatioLimit > 0 && model.SeedRatio >= e.config.SeedRatioLimit && selectedBytes(model) > 0 {
		return "seed-ratio"
	}
	if e.config.SeedTimeMinutes > 0 && model.account.SeedNanos >= int64(time.Duration(e.config.SeedTimeMinutes)*time.Minute) {
		return "seed-time"
	}
	return ""
}
func selectedBytes(model *Torrent) int64 {
	var total int64
	for _, file := range model.Files {
		if model.selection[file.Path] && file.Size > 0 {
			if file.Size > math.MaxInt64-total {
				return math.MaxInt64
			}
			total += file.Size
		}
	}
	return total
}
func projectSeedAccount(model *Torrent) {
	model.Uploaded = model.account.Uploaded
	model.SeedSeconds = model.account.SeedNanos / int64(time.Second)
	model.StopReason = model.account.StopReason
	model.SeedingOverride = model.account.Override
	model.SeedRatio = 0
	if total := selectedBytes(model); total > 0 {
		model.SeedRatio = float64(model.Uploaded) / float64(total)
	}
}

// ResumeSeeding deliberately bypasses the configured seeding budgets until the
// next explicit Stop. It retains accounting and selections across restart.
func (e *Engine) ResumeSeeding(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	if !seedPolicyReason(model.account.StopReason) {
		return errors.New("torrent was not stopped by a seeding limit")
	}
	return e.sessionMutationLocked(func() error {
		model.account.Override = true
		model.account.StopReason = ""
		model.wantStart = true
		model.order = e.nextOrderLocked()
		return e.scheduleLocked()
	})
}
