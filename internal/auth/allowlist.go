package auth

import "errors"

// AllowlistConfig controls which email addresses and domains may log in.
// If both Emails and Domains are empty, all logins are denied.
type AllowlistConfig struct {
	Emails  []string
	Domains []string
}

// ErrLoginDenied is returned by HandleCallback when the user's email is not
// in the configured allowlist.
var ErrLoginDenied = errors.New("login denied: email not in allowlist")
