package cli

import (
	"fmt"
	"path/filepath"
	"strings"

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
		cell := fb.GetCell(row, column)
		if cell == refreshCell {
			ui.refreshFilelist()
			return
		}

		selectedFile := cell.Text
		if strings.ToLower(filepath.Ext(selectedFile)) == ".lua" {
			ui.Session.SendCommand(fmt.Sprintf(`dofile("%s")`, selectedFile))
		}
	})

	var selectedCell *cview.TableCell
	fb.SetSelectionChangedFunc(func(row, column int) {
		selectedCell = fb.GetCell(row, column)
		if selectedCell == refreshCell {
			selectedCell = nil
		}
	})

	fb.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyTAB {
			ui.app.SetFocus(ui.input)
		}

	})

	fb.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyDelete && selectedCell != nil {
			selectedFile := selectedCell.Text
			selectedCell = nil
			fb.Select(0, 0)
			ui.commands <- func() {
				err := ui.removeFile(selectedFile)
				if err != nil {
					ui.Printf("Error removing file: %s\n", err)
				} else {
					ui.refreshFilelist()
				}
			}
		}
		return event
	})

	fb.SetCell(0, 0, refreshCell)
	fb.SetFixed(1, 0)
}

func (ui *UI) refreshFilelist() {
	ui.commands <- func() {
		fileList, err := ui.getFileList()
		if err != nil {
			ui.Printf("Error refreshing file list: %s", err)
			return
		}
		ui.updateFilebrowser(fileList)
	}
}

func (ui *UI) updateFilebrowser(list []fileEntry) {
	fb := ui.fileBrowser
	for fb.GetRowCount() > 1 {
		fb.RemoveRow(1)
	}
	r := 0
	for _, entry := range list {
		r++
		fb.SetCellSimple(r, 0, entry.name)
	}
}
