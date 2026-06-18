package production

import "time"

func testTime(minutes int) time.Time {
	return time.Date(2026, 6, 18, 12, minutes, 0, 0, time.UTC)
}
