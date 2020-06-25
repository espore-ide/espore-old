package fileman

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type LuaRpc interface {
	Rpc(luaCode string) ([]byte, error)
}

type Fileman struct {
	s LuaRpc
}

type FileEntry struct {
	Name string
	Size int
}

func New(s LuaRpc) *Fileman {
	return &Fileman{
		s: s,
	}
}

func (fm *Fileman) List() ([]FileEntry, error) {
	r, err := fm.s.Rpc(`return file.list()`)
	if err != nil {
		return nil, err
	}
	var list map[string]int
	if err := json.Unmarshal(r, &list); err != nil {
		return nil, errors.New("Error decoding file list")
	}

	var entries []FileEntry
	for fileName, size := range list {
		entries = append(entries, FileEntry{Name: fileName, Size: size})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.Compare(entries[i].Name, entries[j].Name) < 0
	})

	return entries, nil
}

func (fm *Fileman) Rename(oldName, newName string) error {
	_, err := fm.s.Rpc(fmt.Sprintf("__espore.renameFile('%s', '%s')", oldName, newName))
	return err
}

func (fm *Fileman) Remove(fileName string) error {
	_, err := fm.s.Rpc(fmt.Sprintf("__espore.removeFile('%s')", fileName))
	return err
}
