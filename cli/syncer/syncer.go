package syncer

import (
	"log"
	"time"

	"github.com/radovskyb/watcher"
)

type FilePusher interface {
	PushFile(srcPath, dstName string) error
}

type Config struct {
	SrcPath string
	OnSync  func(srcPath string)
}

type Syncer struct {
	watcher *watcher.Watcher
}

func New(config *Config) (*Syncer, error) {
	w := watcher.New()
	w.SetMaxEvents(1)
	if err := w.AddRecursive(config.SrcPath); err != nil {
		return nil, err
	}
	s := &Syncer{
		watcher: w,
	}

	go func() {
		for {
			select {
			case event := <-w.Event:
				config.OnSync(event.Path)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	go func() {
		// Start the watching process - it'll check for changes every 100ms.
		if err := w.Start(time.Millisecond * 100); err != nil {
			log.Fatalln(err)
		}
	}()

	return s, nil
}

func (s *Syncer) Close() {
	s.watcher.Close()
}
