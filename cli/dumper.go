package cli

import (
	"io"
	"log"

	"github.com/rivo/tview"
)

type Dumper struct {
	R       io.Reader
	W       io.Writer
	dumping bool
	quitC   chan struct{}
}

func (d *Dumper) Dump() {
	d.dumping = true
	d.quitC = make(chan struct{})

	go func() {
		buffer := make([]byte, 1024)
		for d.dumping {
			i, err := d.R.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Fatalf("Error reading socket: %s", err)
				}
			} else {
				d.W.Write([]byte(tview.Escape(string(buffer[:i]))))
			}
		}
		close(d.quitC)
	}()

}

func (d *Dumper) Stop() {
	d.dumping = false
	<-d.quitC
}

func (d *Dumper) Pause(f func() error) error {
	d.Stop()
	defer d.Dump()
	return f()
}
