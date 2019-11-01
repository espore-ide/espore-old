package initializer

import (
	"fmt"
	"os"
	"path/filepath"

	"espore/builder"
	"espore/session"
	"espore/utils"
)

func Initialize_old(session *session.Session) error {

	chipID, err := session.GetChipID()
	if err != nil {
		return err
	}

	fmt.Printf("Chip ID=%s\n", chipID)

	var manifest builder.FirmwareManifest2
	if err := utils.ReadJSON(filepath.Join("dist", chipID+".json"), &manifest); err != nil {
		return err
	}

	for _, entry := range manifest.Files {
		fmt.Printf("Uploading %s ...", entry.Path)
		if err := session.PushFile(filepath.Join("dist", entry.Base, entry.Path), entry.Path); err != nil {
			return err
		}
	}

	return nil
}

func Initialize(session *session.Session) error {

	chipID, err := session.GetChipID()
	if err != nil {
		return err
	}

	fwFile := filepath.Join("dist", fmt.Sprintf("%s.img", chipID))
	if _, err = os.Stat(fwFile); err != nil {
		fwFile = filepath.Join("dist", "DEFAULT.img")
	}

	err = session.PushFile(fwFile, "update.img")
	if err != nil {
		return err
	}
	err = session.PushFile("bootloader/init.lua", "init.lua")
	if err != nil {
		return err
	}
	return session.NodeRestart()
}
