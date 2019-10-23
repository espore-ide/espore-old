package builder

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"espore/utils"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gobwas/glob"
)

type DeviceInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type FirmwareRoot struct {
	BasePath string
	Files    map[string]*FileEntry
}

type FileEntry struct {
	Base         string   `json:"base"`
	Path         string   `json:"path"`
	Hash         string   `json:"hash"`
	Dependencies []string `json:"-"`
	Datafiles    []string
}

type LibDef struct {
	Name       string   `json:"name"`
	IncludeLua bool     `json:"includeLua"`
	Include    []string `json:"include"`
}
type ModuleDef struct {
	Name      string `json:"name"`
	Autostart bool   `json:"autostart"`
}

type FirmwareDef struct {
	DeviceInfo
	NodeMCUFirmware string      `json:"nodemcu-firmware"`
	Libs            []LibDef    `json:"libs"`
	Modules         []ModuleDef `json:"modules"`
}

type FirmwareManifest2 struct {
	DeviceInfo
	NodeMCUFirmware string
	Files           []*FileEntry `json:"files"`
}

var parseDepRegex = []*regexp.Regexp{
	regexp.MustCompile(`(?m)pcall\s*\(\s*require\s*,\s*"([^"]*)"\s*\)`),
	regexp.MustCompile(`(?m)(?:^require|\s+require|pkg\.require)\s*\(\s*"([^"]*)"\s*(,.*)?\)`),
}
var parseDFRegex = regexp.MustCompile(`(?m)^--\s*datafile:\s*(.*)$`)

func ReadDependenciesAndDatafiles(luaFile string) (deps, datafiles []string, err error) {
	code, err := ioutil.ReadFile(luaFile)
	if err != nil {
		return nil, nil, err
	}
	depMap := make(map[string]bool)
	for _, regex := range parseDepRegex {
		matches := regex.FindAllStringSubmatch(string(code), -1)
		if matches != nil {
			for _, match := range matches {
				depMap[match[1]] = true
			}
		}
	}

	dfMap := make(map[string]bool)
	matches := parseDFRegex.FindAllStringSubmatch(string(code), -1)
	if matches != nil {
		for _, match := range matches {
			dfMap[match[1]] = true
		}
	}

	for dep := range depMap {
		deps = append(deps, dep)
	}

	for df := range dfMap {
		datafiles = append(datafiles, df)
	}

	return deps, datafiles, nil
}

func AddRoot(path string, roots map[string]FirmwareRoot) error {
	list, err := utils.EnumerateDir(path)
	if err != nil {
		return err
	}
	entries := make(map[string]*FileEntry)
	for _, f := range list {
		var entry FileEntry
		fpath := filepath.Join(path, f)
		entry.Path = f
		entry.Base = path
		entry.Hash, err = utils.HashFile(fpath)
		if err != nil {
			return err
		}
		if filepath.Ext(f) == ".lua" {
			deps, datafiles, err := ReadDependenciesAndDatafiles(fpath)
			if err != nil {
				return err
			}
			entry.Dependencies = deps
			entry.Datafiles = datafiles
		}
		entries[entry.Path] = &entry
	}
	roots[path] = FirmwareRoot{
		BasePath: path,
		Files:    entries,
	}
	return nil
}

func getDeviceFirmwareRoots(allRoots map[string]FirmwareRoot, libs []LibDef) ([]FirmwareRoot, error) {
	var roots []FirmwareRoot
	for _, libDef := range libs {
		if libDef.IncludeLua {
			root, ok := allRoots[filepath.Join("site/lib", libDef.Name)]
			if !ok {
				return nil, fmt.Errorf("Cannot find library with name '%s'", libDef.Name)
			}
			roots = append(roots, root)
		}
	}
	return append(roots, allRoots["firmware"]), nil
}

func Mod2File(moduleName string) string {
	return strings.ReplaceAll(moduleName, ".", "/") + ".lua"
}

var ErrFileEntryNotFound = errors.New("Cannot find file in firmware roots")

func FindInRoots(fileName string, roots []FirmwareRoot) (*FileEntry, error) {
	for _, root := range roots {
		entry, ok := root.Files[fileName]
		if ok {
			return entry, nil
		}
	}
	return nil, ErrFileEntryNotFound
}

func AddFilesFromModule(moduleName string, roots []FirmwareRoot, fileMap map[string]*FileEntry) error {
	moduleFileName := Mod2File(moduleName)
	if _, ok := fileMap[moduleFileName]; ok {
		return nil
	}
	entry, err := FindInRoots(moduleFileName, roots)
	if err != nil {
		return fmt.Errorf("Error finding %s: %s", moduleFileName, err)
	}
	fileMap[moduleFileName] = entry
	for _, dep := range entry.Dependencies {
		if err := AddFilesFromModule(dep, roots, fileMap); err != nil {
			return fmt.Errorf("Cannot resolve dependency %q of %s: %s", dep, entry.Path, err)
		}
	}
	return nil
}

func AddOtherFiles(allRoots map[string]FirmwareRoot, libs []LibDef, fileMap map[string]*FileEntry) error {
	for _, lib := range libs {
		for _, pattern := range lib.Include {
			g, err := glob.Compile(pattern, '/')
			if err != nil {
				return fmt.Errorf("Error in glob expression, library %s: %s", lib.Name, err)
			}
			libRoot, ok := allRoots[filepath.Join("site/lib", lib.Name)]
			if !ok {
				return fmt.Errorf("Cannot find library %s: %s", lib.Name, err)
			}
			for path, entry := range libRoot.Files {
				if g.Match(path) {
					fileMap[path] = entry
				}
			}
		}
	}
	return nil
}

func AddDeviceSpecificFiles(deviceRoot *FirmwareRoot, fileMap map[string]*FileEntry) {
	for _, fe := range deviceRoot.Files {
		fileMap[fe.Path] = fe
	}
}

func buildDeviceFirmwareManifest(allRoots map[string]FirmwareRoot, deviceName string) (*FirmwareManifest2, error) {
	var fwDef FirmwareDef
	devicePath := filepath.Join("site/devices", deviceName)
	if err := utils.ReadJSON(filepath.Join(devicePath, "firmware.json"), &fwDef); err != nil {
		return nil, fmt.Errorf("Cannot read firmware file for %s: %s", deviceName, err)
	}
	roots, err := getDeviceFirmwareRoots(allRoots, fwDef.Libs)
	if err != nil {
		return nil, fmt.Errorf("Cannot build firmware roots for %s: %s", deviceName, err)
	}

	fileMap := make(map[string]*FileEntry)
	for _, modDef := range fwDef.Modules {
		if err := AddFilesFromModule(modDef.Name, roots, fileMap); err != nil {
			return nil, fmt.Errorf("Cannot add files from module %s: %s", modDef.Name, err)
		}
	}

	if err := AddOtherFiles(allRoots, fwDef.Libs, fileMap); err != nil {
		return nil, fmt.Errorf("Error adding other files in device %s: %s", deviceName, err)
	}

	deviceRoot, ok := allRoots[devicePath]
	if !ok {
		return nil, fmt.Errorf("Cannot find device root for %s", deviceName)
	}
	AddDeviceSpecificFiles(&deviceRoot, fileMap)

	var manifest FirmwareManifest2
	manifest.DeviceInfo = fwDef.DeviceInfo
	manifest.Name = fwDef.Name
	manifest.Files = make([]*FileEntry, 0, len(fileMap))
	for _, file := range fileMap {
		manifest.Files = append(manifest.Files, file)
	}
	manifest.NodeMCUFirmware = fwDef.NodeMCUFirmware
	return &manifest, nil
}

func writeFileToImage(imageFile io.Writer, path string, size int64, sourceFile io.Reader) error {
	fmt.Fprintln(imageFile, path)
	fmt.Fprintln(imageFile, size)
	_, err := io.Copy(imageFile, sourceFile)
	return err
}

func writeFirmwareImage(manifest *FirmwareManifest2) error {
	imgFilename := filepath.Join("dist", fmt.Sprintf("%s.img", manifest.ID))
	imgFile, err := os.Create(imgFilename)
	if err != nil {
		return err
	}
	defer imgFile.Close()
	var datafiles = []string{} // init like this so when converting to JSON we get an empty array
	var imgBuf = &bytes.Buffer{}
	fmt.Fprintln(imgBuf, "Version: 1 -- HomeNode Device Image File")
	fmt.Fprintf(imgBuf, "Device Id: %s\n", manifest.ID)
	fmt.Fprintf(imgBuf, "Device Name: %s\n", manifest.Name)
	fmt.Fprintf(imgBuf, "Total files: %d\n", len(manifest.Files)+1)
	fmt.Fprintln(imgBuf)

	// sort the files alphabetically to avoid variations in order that would affect
	// the checksum
	sort.Slice(manifest.Files, func(i, j int) bool {
		return strings.Compare(manifest.Files[i].Path, manifest.Files[j].Path) < 0
	})

	for _, fe := range manifest.Files {
		err := func() error {
			path := filepath.Join(fe.Base, fe.Path)
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				return err
			}
			if err := writeFileToImage(imgBuf, fe.Path, fi.Size(), f); err != nil {
				return err
			}
			datafiles = append(datafiles, fe.Datafiles...)
			return nil
		}()
		if err != nil {
			return err
		}
	}
	datafilesJSON, err := json.Marshal(datafiles)
	if err := writeFileToImage(imgBuf, "datafiles.json", int64(len(datafilesJSON)), bytes.NewReader(datafilesJSON)); err != nil {
		return err
	}

	var hash string
	hasher := sha1.New()
	hasher.Write(imgBuf.Bytes())
	hash = hex.EncodeToString(hasher.Sum(nil))
	fmt.Fprintf(imgFile, "Checksum: %s\n", hash)
	_, err = io.Copy(imgFile, imgBuf)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(imgFilename+".hash", []byte(hash), 0666); err != nil {
		return err
	}

	if manifest.NodeMCUFirmware != "" {
		binFilename := fmt.Sprintf("dist/%s.bin", manifest.ID)
		hash, err = utils.CopyFile(filepath.Join("site/nodemcu-firmware", manifest.NodeMCUFirmware), binFilename, true)
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(binFilename+".hash", []byte(hash), 0666)
	}

	return err
}

func Build() error {
	if err := utils.RemoveDirContents("dist"); err != nil {
		return fmt.Errorf("cannot remove dist dir contents: %s", err)
	}

	roots := make(map[string]FirmwareRoot)
	err := AddRoot("firmware", roots)
	if err != nil {
		return err
	}

	var siteLibs []os.FileInfo
	if siteLibs, err = ioutil.ReadDir("site/lib"); err != nil {
		return err
	}
	for _, fd := range siteLibs {
		if fd.IsDir() {
			err = AddRoot(filepath.Join("site/lib", fd.Name()), roots)
			if err != nil {
				return err
			}
		}
	}

	var deviceLibs []os.FileInfo
	if deviceLibs, err = ioutil.ReadDir("site/devices"); err != nil {
		return err
	}
	for _, fd := range deviceLibs {
		if fd.IsDir() {
			err = AddRoot(filepath.Join("site/devices", fd.Name()), roots)
			manifest, err := buildDeviceFirmwareManifest(roots, fd.Name())
			if err != nil {
				return err
			}
			if err := utils.WriteJSON(filepath.Join("dist", manifest.ID+".json"), manifest); err != nil {
				return err
			}
			if err := writeFirmwareImage(manifest); err != nil {
				return err
			}
		}
	}

	return nil
}
