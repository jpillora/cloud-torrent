package engine

import (
	"testing"

	"github.com/anacrolix/torrent"
)

func TestDisableEncryptionConfiguresPeerPolicy(t *testing.T) {
	e := New()
	var applied []torrent.HeaderObfuscationPolicy
	e.newClient = func(c *torrent.ClientConfig) (*torrent.Client, error) {
		applied = append(applied, c.HeaderObfuscationPolicy)
		return torrent.NewClient(c)
	}
	c := mustConfigureEngine(t, e)
	for _, disabled := range []bool{true, false} {
		c.DisableEncryption = disabled
		if err := e.Configure(c); err != nil {
			t.Fatal(err)
		}
		if e.Config().DisableEncryption != disabled {
			t.Fatal("configured policy was not retained")
		}
	}
	if len(applied) < 3 {
		t.Fatalf("expected policy changes to replace client: %v", applied)
	}
	// Bind retries can instantiate the same policy more than once.
	var transitions []torrent.HeaderObfuscationPolicy
	for _, p := range applied {
		if len(transitions) == 0 || transitions[len(transitions)-1] != p {
			transitions = append(transitions, p)
		}
	}
	want := []torrent.HeaderObfuscationPolicy{
		{Preferred: true},
		{Preferred: false, RequirePreferred: true},
		{Preferred: true},
	}
	if len(transitions) != len(want) {
		t.Fatalf("policy transitions: %v", transitions)
	}
	for i := range want {
		if transitions[i] != want[i] {
			t.Fatalf("transition %d: got %+v want %+v", i, transitions[i], want[i])
		}
	}
}
