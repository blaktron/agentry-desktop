package voice

import "time"

// Sync is how this computer's voice stands against the account's, for the
// Local Voice page's Download Cloud version and Upload to Cloud (plan
// voice-page-redesign.md §8, D2; agentry-desktop#610). Newer and older are
// judged with no backend change: equal digests are the same voice;
// otherwise this computer's updated stamp (a download stamps it too)
// against the account version's createdAt.
//
// The known gap (§8): a Restore by swap in the account keeps the restored
// version's old createdAt, so it can look older than it is.
type Sync struct {
	// DownloadSame says this computer keeps the live version.
	DownloadSame bool `json:"downloadSame"`
	// UploadSame says this computer keeps the account's newest version
	// (the pending one, else the live one).
	UploadSame bool `json:"uploadSame"`
	// LocalAfterLive says this computer's voice was saved after the live
	// version was set, so a download would replace a newer voice.
	LocalAfterLive bool `json:"localAfterLive"`
	// AccountAfterLocal says the account's newest version was set after
	// this computer's voice was saved, so an upload would follow a newer one.
	AccountAfterLocal bool `json:"accountAfterLocal"`
	// InReview says a version newer than the live one is in AI Review and
	// not live yet.
	InReview bool `json:"inReview"`
}

// Newest is the account's newest version: the pending one, else the live
// one; nil when it keeps none.
func (a *Account) Newest() *Version {
	if a == nil {
		return nil
	}
	if a.Pending != nil {
		return a.Pending
	}
	return a.Live
}

// UpdatedAt is when this computer's voice was last saved here; false
// when it keeps none or its record's stamp does not read.
func (l *Local) UpdatedAt() (time.Time, bool) {
	if l == nil || !l.Exists {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, l.Updated)
	return t, err == nil
}

// SyncOf compares this computer's voice with the account's.
func SyncOf(l *Local, a *Account) Sync {
	var s Sync
	if a == nil {
		return s
	}
	here := l != nil && l.Exists
	if live := a.Live; live != nil {
		if p := a.Pending; p != nil && p.Review == "reviewing" && p.CreatedAt.After(live.CreatedAt) {
			s.InReview = true
		}
		if here {
			s.DownloadSame = l.Digest == live.Digest
			if t, ok := l.UpdatedAt(); ok && !s.DownloadSame {
				s.LocalAfterLive = t.After(live.CreatedAt)
			}
		}
	}
	if n := a.Newest(); n != nil && here {
		s.UploadSame = l.Digest == n.Digest
		if t, ok := l.UpdatedAt(); ok && !s.UploadSame {
			s.AccountAfterLocal = n.CreatedAt.After(t)
		}
	}
	return s
}
