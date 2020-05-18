package cli

import (
	"github.com/gdamore/tcell"
	"gitlab.com/tslocum/cview"
)

func (ui *UI) initFileBrowser() {
	fb := ui.fileBrowser
	fb.SetBorder(true)
	fb.SetSelectable(true, true)

	refreshCell := cview.NewTableCell("(Refresh)").
		SetAlign(cview.AlignCenter).SetTextColor(tcell.ColorYellow)

	fb.SetSelectedFunc(func(row, column int) {
		if fb.GetCell(row, column) == refreshCell {
			ui.commands <- func() {
				fileList, err := ui.getFileList()
				if err != nil {
					ui.Printf("Error refreshing file list: %s", err)
					return
				}
				ui.updateFileList(fileList)

			}
		}
	})

	fb.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyTAB {
			ui.app.SetFocus(ui.input)
		}

	})

	fb.SetCell(0, 0, refreshCell)
	fb.SetFixed(1, 0)
}

func (ui *UI) updateFileList(fileList map[string]int) {
	fb := ui.fileBrowser
	for fb.GetRowCount() > 1 {
		fb.RemoveRow(1)
	}
	r := 0
	for fileName := range fileList {
		r++
		fb.SetCellSimple(r, 0, fileName)
	}
}
