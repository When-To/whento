// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package logger

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// fingerprintSalt keys the HMAC behind Fingerprint. It is random per process and
// never written anywhere, which is what makes a fingerprint useless outside the
// run that produced it: an archive of log lines cannot be matched back to an
// email address or a calendar token by hashing candidates, because there is no
// salt to hash them with.
//
// The cost of that choice is deliberate. Fingerprints do not correlate across a
// restart or across two instances. Correlating a single incident inside one run
// is what an operator actually needs from these lines; building a stable
// identifier for a person over time is exactly what must not be possible.
var fingerprintSalt = newFingerprintSalt()

func newFingerprintSalt() []byte {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand does not fail on any platform Go supports. Refusing to
		// start beats logging under a predictable all-zero salt.
		panic("logger: cannot generate the fingerprint salt: " + err.Error())
	}

	return salt
}

// fingerprintLength is the number of HMAC bytes kept, hex-encoded to twice as
// many characters. Eight bytes is far more than enough to tell two subjects
// apart inside one run's worth of log lines, and short enough that the field
// reads as an opaque tag rather than as something worth trying to reverse.
const fingerprintLength = 8

// Fingerprint turns a value that must never appear in a log line — an email
// address, a calendar token, a participant id — into a short correlation tag.
//
// The tag says "these lines are about the same subject" and nothing else. It is
// not reversible, it is not stable beyond the life of the process, and it is not
// comparable with a tag from any other process. Use it where a log line would
// otherwise be useless without some way to group it; where an internal user id
// is already at hand, prefer that, since it is what the access log already
// carries for authenticated requests.
//
// An empty value yields an empty tag rather than the fingerprint of the empty
// string, so a missing field cannot be mistaken for a real subject.
func Fingerprint(value string) string {
	if value == "" {
		return ""
	}

	mac := hmac.New(sha256.New, fingerprintSalt)
	mac.Write([]byte(value))

	return hex.EncodeToString(mac.Sum(nil)[:fingerprintLength])
}
