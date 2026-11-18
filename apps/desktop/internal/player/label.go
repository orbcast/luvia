package player

import "time"

// LengthLabel is the total-time reading the controls show: a dash while the
// engine has not learned the length, otherwise the media time.
func LengthLabel(total time.Duration) string { return lengthLabel(total) }
