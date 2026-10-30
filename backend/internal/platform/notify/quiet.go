package notify

import "time"

// QuietHours holds back non-urgent SMS and LINE messages overnight in the recipient's local time
// (Asia/Bangkok). E-mail and in-app are not intrusive and are never held. Not decided in
// docs/decisions.md — the window is configuration (DefaultQuietHours).
type QuietHours struct {
	Enabled   bool
	StartHour int // local hour the quiet period starts, e.g. 21
	EndHour   int // local hour it ends, e.g. 8
	Location  *time.Location
}

// DefaultQuietHours: 21:00–08:00 Asia/Bangkok.
func DefaultQuietHours() QuietHours {
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	return QuietHours{Enabled: true, StartHour: 21, EndHour: 8, Location: loc}
}

// Release returns when a message queued at now may be sent: now itself outside the window, else the
// end of the current quiet period.
func (q QuietHours) Release(now time.Time, channel string, urgent bool) time.Time {
	if !q.Enabled || urgent || (channel != ChannelSMS && channel != ChannelLine) {
		return now
	}
	local := now.In(q.Location)
	h := local.Hour()
	inWindow := false
	if q.StartHour > q.EndHour { // wraps midnight
		inWindow = h >= q.StartHour || h < q.EndHour
	} else {
		inWindow = h >= q.StartHour && h < q.EndHour
	}
	if !inWindow {
		return now
	}
	end := time.Date(local.Year(), local.Month(), local.Day(), q.EndHour, 0, 0, 0, q.Location)
	if !end.After(local) {
		end = end.AddDate(0, 0, 1)
	}
	return end.UTC()
}
