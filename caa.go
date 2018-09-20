package jwt

import "time"

type SessionCAA int64

type CAA interface {
	Lock()
	Unlock()
	IsLocked() bool

	IsValid(SessionCAA, int64) bool

	Revoke(int64)
	Issue() SessionCAA
	HasIssued() bool
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

var (
	// Force to UTC so no possible timzone conflicts across servers
	Now = func() time.Time { return time.Now().UTC() }
)

func NowForce(t time.Time) {
	Now = func() time.Time { return t }
}

func NowReset() {
	Now = func() time.Time { return time.Now().UTC() }
}
