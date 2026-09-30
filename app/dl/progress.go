package dl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gabriel-vasile/mimetype"
	"github.com/go-faster/errors"
	pw "github.com/jedib0t/go-pretty/v6/progress"

	"github.com/iyear/tdl/core/downloader"
	"github.com/iyear/tdl/core/util/fsutil"
	"github.com/iyear/tdl/pkg/prog"
	"github.com/iyear/tdl/pkg/utils"
)

type progress struct {
	pw       pw.Writer
	trackers *sync.Map // map[ID]*pw.Tracker
	opts     Options

	it   *iter
	hook Hook
}

func newProgress(p pw.Writer, it *iter, opts Options) *progress {
	return &progress{
		pw:       p,
		trackers: &sync.Map{},
		opts:     opts,
		it:       it,
		hook:     opts.Hook,
	}
}

func (p *progress) snapshot(e *iterElem) ElemInfo {
	return ElemInfo{
		ID:        e.id,
		Name:      strings.TrimSuffix(filepath.Base(e.to.Name()), tempExt),
		Path:      e.to.Name(),
		Size:      e.file.Size,
		DialogID:  e.from.ID(),
		MessageID: e.fromMsg.ID,
	}
}

func (p *progress) OnAdd(elem downloader.Elem) {
	tracker := prog.AppendTracker(p.pw, utils.Byte.FormatBinaryBytes, p.processMessage(elem), elem.File().Size())
	p.trackers.Store(elem.(*iterElem).id, tracker)

	if p.hook != nil {
		p.hook.OnAdd(p.snapshot(elem.(*iterElem)))
	}
}

func (p *progress) OnDownload(elem downloader.Elem, state downloader.ProgressState) {
	tracker, ok := p.trackers.Load(elem.(*iterElem).id)
	if !ok {
		return
	}

	t := tracker.(*pw.Tracker)
	t.UpdateTotal(state.Total)
	t.SetValue(state.Downloaded)

	if p.hook != nil {
		p.hook.OnDownload(p.snapshot(elem.(*iterElem)), state.Downloaded, state.Total)
	}
}

func (p *progress) OnDone(elem downloader.Elem, err error) {
	e := elem.(*iterElem)

	tracker, ok := p.trackers.Load(e.id)
	if !ok {
		return
	}
	t := tracker.(*pw.Tracker)

	rerr := err // effective error reported to hook
	finalPath := ""
	defer func() {
		if p.hook != nil {
			info := p.snapshot(e)
			info.FinalPath = finalPath
			p.hook.OnDone(info, rerr)
		}
	}()

	if err := e.to.Close(); err != nil {
		err = errors.Wrap(err, "close file")
		p.fail(t, elem, err)
		rerr = err
		return
	}

	if err != nil {
		if !errors.Is(err, context.Canceled) { // don't report user cancel
			p.fail(t, elem, errors.Wrap(err, "progress"))
		}
		_ = os.Remove(e.to.Name()) // just try to remove temp file, ignore error
		return
	}

	p.it.Finish(e.logicalPos)

	newPath, err := p.donePost(e)
	if err != nil {
		err = errors.Wrap(err, "post file")
		p.fail(t, elem, err)
		rerr = err
		return
	}
	finalPath = newPath
}

func (p *progress) donePost(elem *iterElem) (string, error) {
	newfile := strings.TrimSuffix(filepath.Base(elem.to.Name()), tempExt)

	if p.opts.RewriteExt {
		mime, err := mimetype.DetectFile(elem.to.Name())
		if err != nil {
			return "", errors.Wrap(err, "detect mime")
		}
		ext := mime.Extension()
		if ext != "" && (filepath.Ext(newfile) != ext) {
			newfile = fsutil.GetNameWithoutExt(newfile) + ext
		}
	}

	newpath := filepath.Join(filepath.Dir(elem.to.Name()), newfile)
	if err := os.Rename(elem.to.Name(), newpath); err != nil {
		return "", errors.Wrap(err, "rename file")
	}

	// Set file modification time to message date if available
	if elem.file.Date > 0 {
		fileTime := time.Unix(elem.file.Date, 0)
		if err := os.Chtimes(newpath, fileTime, fileTime); err != nil {
			return newpath, errors.Wrap(err, "set file time")
		}
	}

	return newpath, nil
}

func (p *progress) fail(t *pw.Tracker, elem downloader.Elem, err error) {
	if p.hook == nil {
		p.pw.Log(color.RedString("%s error: %s", p.elemString(elem), err.Error()))
	}
	t.MarkAsErrored()
}

func (p *progress) processMessage(elem downloader.Elem) string {
	return p.elemString(elem)
}

func (p *progress) elemString(elem downloader.Elem) string {
	e := elem.(*iterElem)
	return fmt.Sprintf("%s(%d):%d -> %s",
		e.from.VisibleName(),
		e.from.ID(),
		e.fromMsg.ID,
		strings.TrimSuffix(e.to.Name(), tempExt))
}
