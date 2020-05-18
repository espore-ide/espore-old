package cli

func (ui *UI) initFileBrowser() {
	ui.fileBrowser.SetBorder(true)
	ui.fileBrowser.SetCellSimple(0, 0, "E")
	ui.fileBrowser.SetCellSimple(0, 1, "R")
	ui.fileBrowser.SetCellSimple(0, 2, "File")
}
