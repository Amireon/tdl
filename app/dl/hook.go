package dl

// ElemInfo is a snapshot of a download element exposed to Hook
// implementations, so that callers (e.g. GUI) don't need to access
// internal types like iterElem, tmedia.Media or peers.Peer.
type ElemInfo struct {
	ID        int
	Name      string // file name without temp extension
	Path      string // full target path (with temp extension)
	FinalPath string // actual path after download (MIME ext rewrite, etc.); empty until OnDone
	Size      int64
	DialogID  int64
	MessageID int
}

// Hook subscribes to download events. It must be safe for concurrent use.
// When Options.Hook is set, terminal rendering is disabled and events are
// forwarded to the hook instead.
type Hook interface {
	OnAdd(elem ElemInfo)
	OnDownload(elem ElemInfo, downloaded, total int64)
	OnDone(elem ElemInfo, err error)
}
