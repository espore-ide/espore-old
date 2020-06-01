package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (ui *UI) Rpc(luaCode string) ([]byte, error) {
	ui.dumper.Stop()
	defer ui.dumper.Dump()
	return ui.Session.Rpc(luaCode)
}

type fileEntry struct {
	name string
	size int
}

func (ui *UI) getFileList() ([]fileEntry, error) {
	r, err := ui.Rpc(`return file.list()`)
	if err != nil {
		return nil, err
	}
	var list map[string]int
	if err := json.Unmarshal(r, &list); err != nil {
		return nil, errors.New("Error decoding file list")
	}

	var entries []fileEntry
	for fileName, size := range list {
		entries = append(entries, fileEntry{name: fileName, size: size})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.Compare(entries[i].name, entries[j].name) < 0
	})

	return entries, nil
}

func (ui *UI) removeFile(fileName string) error {
	_, err := ui.Rpc(fmt.Sprintf("__espore.removeFile('%s')", fileName))
	return err
}

func (ui *UI) renameFile(oldName, newName string) error {
	_, err := ui.Rpc(fmt.Sprintf("__espore.renameFile('%s', '%s')", oldName, newName))
	return err
}
