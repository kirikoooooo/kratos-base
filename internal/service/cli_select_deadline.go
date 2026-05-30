package service

import (
	"time"
)

func deadlineSoon() time.Time {
	return time.Now().Add(50 * time.Millisecond)
}

func noDeadline() time.Time {
	return time.Time{}
}
