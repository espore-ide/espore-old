package initializer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"espore/session"
)

func Initialize(outputDir string, session *session.Session) error {
	chipID, err := session.GetChipID()
	if err != nil {
		return err
	}

	fwFile := filepath.Join(outputDir, fmt.Sprintf("%s.img", chipID))
	if _, err = os.Stat(fwFile); err != nil {
		fwFile = filepath.Join(outputDir, "DEFAULT.img")
	}
	err = session.PushFile(fwFile, "update.img")
	if err != nil {
		return err
	}
	err = session.PushStream(strings.NewReader(InitLua), int64(len(InitLua)), "init.lua")
	if err != nil {
		return err
	}
	return session.NodeRestart()
}
