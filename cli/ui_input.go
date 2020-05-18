package cli

import (
	"strings"

	"github.com/gdamore/tcell"
)

var history []string
var historyPos int

func (ui *UI) initInput() {
	input := ui.input
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyTAB:
			ui.app.SetFocus(ui.output)
		case tcell.KeyEnter:
			cmd := strings.TrimSpace(input.GetText())
			if len(cmd) == 0 {
				return
			}
			ui.input.SetText("")
			ui.commands <- func() {
				err := ui.parseCommandLine(cmd)
				if err != nil {
					ui.Printf("Error executing command: %s", err)
				}
			}
			lh := len(history)
			if lh == 0 || (lh > 0 && history[lh-1] != cmd) {
				history = append(history, cmd)
				historyPos = lh + 1
			}
		}
	})

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			if historyPos > 0 {
				historyPos--
				input.SetText(history[historyPos])
			}
			return nil
		case tcell.KeyDown:
			if historyPos < len(history)-1 {
				historyPos++
				input.SetText(history[historyPos])
			} else {
				input.SetText("")
			}
			return nil
		}
		return event
	})
}
