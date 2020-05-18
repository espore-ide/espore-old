package cli

import "github.com/gdamore/tcell"

func (ui *UI) initOutput() {
	output := ui.output
	output.
		SetDynamicColors(true).
		SetRegions(true).
		SetWordWrap(true).
		SetMaxLines(300).
		SetScrollable(true).
		ScrollToEnd().SetBorder(true)

	output.SetChangedFunc(func() {
		ui.app.Draw()
	})

	output.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyTAB {
			ui.app.SetFocus(ui.input)
		}

	})
}
