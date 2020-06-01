package cli

import "github.com/rivo/tview"

func (ui *UI) initLayout() {

	ui.innerFlex.SetDirection(tview.FlexColumn)
	ui.innerFlex.AddItem(ui.output, 0, 1, false)
	ui.innerFlex.AddItem(ui.fileBrowser, 20, 0, false)

	ui.outerFlex.SetDirection(tview.FlexRow)
	ui.outerFlex.AddItem(ui.innerFlex, 0, 1, false)
	ui.outerFlex.AddItem(ui.input, 1, 0, true)

	ui.mainWnd.SetRoot(ui.outerFlex)
}
