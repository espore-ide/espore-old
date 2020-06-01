package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/epiclabs-io/winman"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

func (ui *UI) initFileBrowser() {
	fb := ui.fileBrowser
	fb.SetBorder(true)
	fb.SetSelectable(true, true)

	refreshCell := tview.NewTableCell("(Refresh)").
		SetAlign(tview.AlignCenter).SetTextColor(tcell.ColorYellow)

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

	var selectedCell *tview.TableCell
	fb.SetSelectionChangedFunc(func(row, column int) {
		if row < 0 || column < 0 {
			return
		}
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
		if selectedCell == nil {
			return event
		}

		selectedFile := selectedCell.Text
		switch event.Key() {
		case tcell.KeyDelete:
			selectedCell = nil
			fb.Select(0, 0)
			ui.commands <- func() {
				ui.Printf("Deleting %s ... ", selectedFile)
				err := ui.removeFile(selectedFile)
				if err != nil {
					ui.Printf("ERROR: %s\n", err)
					return
				}
				ui.refreshFilelist()
				ui.Printf("OK\n")
			}
		case tcell.KeyF2:
			var renameWnd winman.Window
			renameWnd = renameDialog(selectedFile, func(newName string) {
				ui.wm.RemoveWindow(renameWnd)
				ui.app.SetFocus(ui.fileBrowser)
				if newName == "" {
					return
				}
				ui.commands <- func() {
					ui.Printf("Renaming %s to %s ...", selectedFile, newName)
					err := ui.renameFile(selectedFile, newName)
					if err != nil {
						ui.Printf("ERROR: %s\n", err)
						return
					}
					ui.refreshFilelist()
					ui.Printf("OK\n")
				}
			})
			ui.wm.AddWindow(renameWnd)
			ui.wm.Center(renameWnd)
			ui.app.SetFocus(renameWnd)
		}
		return event
	})

	fb.SetCell(0, 0, refreshCell)
	fb.SetFixed(1, 0)
}

func (ui *UI) refreshFilelist() {
	ui.commands <- func() {
		ui.Printf("Retrieving file list ... ")
		fileList, err := ui.getFileList()
		if err != nil {
			ui.Printf("ERROR: %s\n", err)
			return
		}
		ui.updateFilebrowser(fileList)
		ui.Printf("OK\n")
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
	ui.app.Draw()
}

func renameDialog(oldName string, callback func(newName string)) winman.Window {
	form := tview.NewForm()
	newName := tview.NewInputField().
		SetLabel("New name:").
		SetText(oldName).
		SetFieldWidth(20)

	form.AddFormItem(newName).
		AddButton("OK", func() {
			callback(newName.GetText())
		}).
		AddButton("Cancel", func() {
			callback("")
		})
	wnd := winman.NewWindow().
		SetRoot(form).
		SetModal(true).
		SetResizable(true).
		SetDraggable(true).
		Show()

	wnd.SetRect(0, 0, 30, 10)
	return wnd
}
