package cli

import "gitlab.com/tslocum/cview"

func (ui *UI) initLayout() {

	ui.innerFlex.SetDirection(cview.FlexColumn)
	ui.innerFlex.AddItem(ui.output, 0, 1, false)
	ui.innerFlex.AddItem(ui.fileBrowser, 20, 0, false)

	ui.outerFlex.SetDirection(cview.FlexRow)
	ui.outerFlex.AddItem(ui.innerFlex, 0, 1, false)
	ui.outerFlex.AddItem(ui.input, 1, 0, true)
}
