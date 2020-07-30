package builder

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"espore/config"
	"espore/initializer"
	"espore/session"
	"espore/utils"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/gobwas/glob"
)

type DeviceInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type FirmwareLib struct {
	BasePath     string
	Files        map[string]*FileEntry
	Modules      []ModuleDef `json:"modules"`
	Dependencies []*FirmwareLib
}

type FileEntry struct {
	Base         string   `json:"base"`
	Path         string   `json:"path"`
	Hash         string   `json:"hash"`
	Dependencies []string `json:"-"`
	Datafiles    []string `json:"datafiles,omitempty"`
	Content      []byte   `json:"-"`
}

type LibDef struct {
	Dependencies []string    `json:"dependencies"`
	Include      []string    `json:"include"`
	Exclude      []string    `json:"exclude`
	Name         string      `json:"name"`
	Modules      []ModuleDef `json:"modules"`
}

type ModuleDef struct {
	Name      string          `json:"name"`
	Autostart bool            `json:"autostart"`
	Config    json.RawMessage `json:"config,omitempty"`
}

type FirmwareLFSConfig struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude`
}

type FirmwareDef struct {
	DeviceInfo
	NodeMCUFirmware string            `json:"nodemcu-firmware"`
	Libs            []string          `json:"libs"`
	LFS             FirmwareLFSConfig `json:"lfs"`
}

type FirmwareManifest struct {
	DeviceInfo
	NodeMCUFirmware string
	Files           []*FileEntry `json:"files"`
}

type LFSEntry struct {
	Files []*FileEntry
	Hash  string
}

var parseDepRegex = []*regexp.Regexp{
	regexp.MustCompile(`(?m)pcall\s*\(\s*require\s*,\s*"([^"]*)"\s*\)`),
	regexp.MustCompile(`(?m)(?:^require|\s+require|pkg\.require)\s*\(\s*"([^"]*)"\s*(,.*)?\)`),
}
var parseDFRegex = regexp.MustCompile(`(?m)^--\s*datafile:\s*(.*)$`)

var LFSEmbeddedFiles = map[string]string{
	"__lfsinit.lua": lfsInitLua,
	"__espore.lua":  session.EsporeLua,
}

func extractFile(file string, contents string, outputDir string) error {
	path := filepath.Join(outputDir, file)
	if _, err := os.Stat(path); err != nil {
		if err := ioutil.WriteFile(path, []byte(contents), 0666); err != nil {
			return fmt.Errorf("Error creating file %s: %s", file, err)
		}
	}
	return nil
}

func Luac(sourceEntries []*FileEntry, dstFile string) (err error) {

	tmpDir, err := ioutil.TempDir("", "espore-luac")
	if err != nil {
		return err
	}
	var sources []string
	for _, f := range sourceEntries {
		dst := strings.ReplaceAll(strings.ReplaceAll(f.Path, "/", ","), "\\", ",")
		dst = filepath.Join(tmpDir, dst)
		utils.CopyFile(filepath.Join(f.Base, f.Path), dst, false)
		sources = append(sources, dst)
	}

	cmd := exec.Command("luac.cross", append([]string{"-o", dstFile, "-f"}, sources...)...)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		exitErr := err.(*exec.ExitError)
		var code int
		if exitErr != nil {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				code = status.ExitStatus()
			}
		}
		return fmt.Errorf("Error compiling lua, error code %d:\n%s", code, outputBytes)
	}
	return nil
}

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

func LoadLibrary(path string, allLibs map[string]*FirmwareLib, level int) (*FirmwareLib, error) {
	lib := allLibs[path]
	if lib != nil {
		return lib, nil
	}
	if level > 100 {
		return nil, fmt.Errorf("Circular dependency in %q", path)
	}

	list, err := utils.EnumerateDir(path)
	if err != nil {
		return nil, err
	}

	var libDef LibDef
	libDefPath := filepath.Join(path, "library.json")
	utils.ReadJSON(libDefPath, &libDef)
	if len(libDef.Include) == 0 {
		libDef.Include = []string{"*"}
	}
	if libDef.Name == "" {
		libDef.Name = path
	}
	var includes []glob.Glob
	var excludes []glob.Glob

	for _, i := range libDef.Include {
		g, err := glob.Compile(i, '/')
		if err != nil {
			return nil, fmt.Errorf("Error parsing include glob in %s", libDefPath)
		}
		includes = append(includes, g)
	}
	for _, e := range libDef.Exclude {
		g, err := glob.Compile(e, '/')
		if err != nil {
			return nil, fmt.Errorf("Error parsing exclude glob in %s", libDefPath)
		}
		excludes = append(excludes, g)
	}

	entries := make(map[string]*FileEntry)
	for _, f := range list {
		if f == "library.json" {
			continue
		}
		var entry FileEntry
		fpath := filepath.Join(path, f)
		entry.Path = f
		entry.Base = path
		entry.Hash, err = utils.HashFile(fpath)
		if err != nil {
			return nil, err
		}
		var add bool
		if isLua(f) {
			add = true
			deps, datafiles, err := ReadDependenciesAndDatafiles(fpath)
			if err != nil {
				return nil, err
			}
			entry.Dependencies = deps
			entry.Datafiles = datafiles
		} else {
			for _, ig := range includes {
				if ig.Match(f) {
					add = true
					break
				}
			}
			for _, eg := range excludes {
				if eg.Match(f) {
					add = false
					break
				}
			}
		}
		if add {
			entries[entry.Path] = &entry
		}
	}

	var dependencies []*FirmwareLib
	for _, depLibName := range libDef.Dependencies {
		dep, err := LoadLibrary(depLibName, allLibs, level+1)
		if err != nil {
			return nil, fmt.Errorf("Error resolving dependency %q of library %q", depLibName, path)
		}
		dependencies = append(dependencies, dep)
	}

	lib = &FirmwareLib{
		BasePath:     path,
		Files:        entries,
		Modules:      libDef.Modules,
		Dependencies: dependencies,
	}
	allLibs[path] = lib
	return lib, nil
}

func getLibraryList(lib *FirmwareLib, added map[*FirmwareLib]bool) []*FirmwareLib {
	if added == nil {
		added = make(map[*FirmwareLib]bool)
	}
	if added[lib] {
		return nil
	}
	var libList []*FirmwareLib
	for _, dep := range lib.Dependencies {
		libList = append(libList, getLibraryList(dep, added)...)
	}
	added[lib] = true
	return append(libList, lib)

}

func Mod2File(moduleName string) string {
	return strings.ReplaceAll(moduleName, ".", "/") + ".lua"
}

var ErrFileEntryNotFound = errors.New("Cannot find file in firmware libraries")

func FindInLibraries(fileName string, libs []*FirmwareLib) (*FileEntry, error) {
	for _, lib := range libs {
		entry, ok := lib.Files[fileName]
		if ok {
			return entry, nil
		}
	}
	return nil, ErrFileEntryNotFound
}

func AddFilesFromModule(moduleName string, libs []*FirmwareLib, fileMap map[string]*FileEntry) error {
	moduleFileName := Mod2File(moduleName)
	if _, ok := fileMap[moduleFileName]; ok {
		return nil
	}
	entry, err := FindInLibraries(moduleFileName, libs)
	if err != nil {
		return fmt.Errorf("Error finding %s: %s", moduleFileName, err)
	}
	fileMap[moduleFileName] = entry
	for _, dep := range entry.Dependencies {
		if err := AddFilesFromModule(dep, libs, fileMap); err != nil {
			return fmt.Errorf("Cannot resolve dependency %q of %s: %s", dep, entry.Path, err)
		}
	}
	return nil
}

func AddOtherFiles(libs []*FirmwareLib, fileMap map[string]*FileEntry) error {
	for _, lib := range libs {
		for path, entry := range lib.Files {
			if !isLua(path) {
				fileMap[path] = entry
			}
		}
	}
	return nil
}

func AddDeviceSpecificFiles(deviceRootLib *FirmwareLib, fileMap map[string]*FileEntry) {
	for _, fe := range deviceRootLib.Files {
		fileMap[fe.Path] = fe
	}
}

func removeDuplicateModules(mods []ModuleDef) []ModuleDef {
	modmap := make(map[string]ModuleDef)
	for _, mod := range mods {
		if _, ok := modmap[mod.Name]; !ok {
			modmap[mod.Name] = mod
		}
	}
	mods = make([]ModuleDef, 0, len(modmap))
	for _, mod := range modmap {
		mods = append(mods, mod)
	}
	sort.SliceStable(mods, func(i, j int) bool {
		return strings.Compare(mods[i].Name, mods[j].Name) < 0
	})
	return mods
}

func NewVirtualFileEntry(data []byte, path string) *FileEntry {
	var fe FileEntry
	fe.Path = path
	fe.Content = data
	hasher := sha1.New()
	hasher.Write(data)
	fe.Hash = hex.EncodeToString(hasher.Sum(nil))
	return &fe
}

var MainModule = ModuleDef{
	Name: "main",
}

func packLFS(manifest *FirmwareManifest, LFSConfig FirmwareLFSConfig) error {
	var lfs LFSEntry
	var files []*FileEntry
	hasher := sha1.New()

	if len(LFSConfig.Include) == 0 {
		LFSConfig.Include = []string{"**/*", "*"}
	}

	LFSConfig.Exclude = append(LFSConfig.Exclude, "init.lua") // always exclude init.lua from LFS

	var includes []glob.Glob
	var excludes []glob.Glob

	for _, i := range LFSConfig.Include {
		g, err := glob.Compile(i, '/')
		if err != nil {
			return fmt.Errorf("Error parsing LFS include glob in %s firmware manifest file", manifest.Name)
		}
		includes = append(includes, g)
	}
	for _, e := range LFSConfig.Exclude {
		g, err := glob.Compile(e, '/')
		if err != nil {
			return fmt.Errorf("Error parsing LFS exclude glob in %s firmware manifest file", manifest.Name)
		}
		excludes = append(excludes, g)
	}

	for _, file := range manifest.Files {
		var add bool
		for _, ig := range includes {
			if ig.Match(file.Path) {
				add = true
				break
			}
		}
		for _, eg := range excludes {
			if eg.Match(file.Path) {
				add = false
				break
			}

		}
		add = add && isLua(file.Path)
		if add {
			lfs.Files = append(lfs.Files, file)
			hasher.Write([]byte(file.Hash))
		} else {
			files = append(files, file)
		}
	}

	manifest.Files = files

	if len(lfs.Files) > 0 {
		lfs.Hash = hex.EncodeToString(hasher.Sum(nil))
		tmpDir, err := ioutil.TempDir("", "espore-luac")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpDir)

		for file, content := range LFSEmbeddedFiles {
			if err := extractFile(file, content, tmpDir); err != nil {
				return err
			}
		}

		for file := range LFSEmbeddedFiles {
			lfs.Files = append(lfs.Files, &FileEntry{
				Base: tmpDir,
				Path: file,
			})
		}

		lfsFile := filepath.Join(tmpDir, fmt.Sprintf("%s.lfs", lfs.Hash))
		if err := Luac(lfs.Files, lfsFile); err != nil {
			return fmt.Errorf("Error compiling lua firmware for %s: %s", manifest.DeviceInfo.Name, err)
		}
		lfsData, err := ioutil.ReadFile(lfsFile)
		if err != nil {
			return fmt.Errorf("Error reading lfs file %s for %s: %s", lfsFile, manifest.DeviceInfo.Name, err)
		}
		lfsFileEntry := NewVirtualFileEntry(lfsData, "lfs.img")
		lfsFileEntry.Hash, err = utils.HashFile(lfsFile)
		if err != nil {
			return fmt.Errorf("Error hasing lfs file %s for %s: %s", lfsFile, manifest.DeviceInfo.Name, err)
		}
		manifest.Files = append(manifest.Files, lfsFileEntry)
	}

	return nil
}

func buildDeviceFirmwareManifest(deviceRootLib *FirmwareLib, fwDef FirmwareDef) (*FirmwareManifest, error) {
	usedLibs := getLibraryList(deviceRootLib, nil)

	var modules []ModuleDef
	modules = append(modules, deviceRootLib.Modules...)
	for _, lib := range usedLibs {
		modules = append(modules, lib.Modules...)
	}
	modules = removeDuplicateModules(modules)
	modules = append(modules, MainModule)

	fileMap := make(map[string]*FileEntry)
	for _, modDef := range modules {
		if err := AddFilesFromModule(modDef.Name, usedLibs, fileMap); err != nil {
			return nil, fmt.Errorf("Cannot add files from module %s: %s. Are you including the library where %s is defined?", modDef.Name, err, modDef.Name)
		}
	}

	if err := AddOtherFiles(usedLibs, fileMap); err != nil {
		return nil, fmt.Errorf("Error adding other files in device %s: %s", fwDef.Name, err)
	}

	AddDeviceSpecificFiles(deviceRootLib, fileMap)

	modbytes, err := json.MarshalIndent(modules, "", "\t")
	if err != nil {
		return nil, err
	}
	fileMap["modules.json"] = NewVirtualFileEntry(modbytes, "modules.json")
	fileMap["init.lua"] = NewVirtualFileEntry([]byte(initializer.InitLua), "init.lua")

	var manifest FirmwareManifest
	manifest.DeviceInfo = fwDef.DeviceInfo
	manifest.Name = fwDef.Name
	manifest.Files = make([]*FileEntry, 0, len(fileMap))
	for _, file := range fileMap {
		manifest.Files = append(manifest.Files, file)
	}
	manifest.NodeMCUFirmware = fwDef.NodeMCUFirmware

	err = packLFS(&manifest, fwDef.LFS)
	if err != nil {
		return nil, err
	}

	return &manifest, nil
}

func writeFileToImage(imageFile io.Writer, path string, size int64, sourceFile io.Reader) error {
	fmt.Fprintln(imageFile, path)
	fmt.Fprintln(imageFile, size)
	_, err := io.Copy(imageFile, sourceFile)
	return err
}

func writeFirmwareImage(manifest *FirmwareManifest, outputDir string) error {

	// sort the files alphabetically to avoid variations in order that would affect
	// the checksum
	sort.Slice(manifest.Files, func(i, j int) bool {
		return strings.Compare(manifest.Files[i].Path, manifest.Files[j].Path) < 0
	})

	var datafiles = []string{} // init like this so when converting to JSON we get an empty array

	for _, fe := range manifest.Files {
		datafiles = append(datafiles, fe.Datafiles...)
	}

	imgFilename := filepath.Join(outputDir, fmt.Sprintf("%s.img", manifest.ID))
	imgFile, err := os.Create(imgFilename)
	if err != nil {
		return err
	}
	defer imgFile.Close()
	var imgBuf = &bytes.Buffer{}
	fmt.Fprintf(imgBuf, "Version: 1 -- ESPore Device Image File\n")
	fmt.Fprintf(imgBuf, "Device Id: %s\n", manifest.ID)
	fmt.Fprintf(imgBuf, "Device Name: %s\n", manifest.Name)
	fmt.Fprintf(imgBuf, "Total files: %d\n", len(manifest.Files)+1)
	fmt.Fprintln(imgBuf)

	for _, fe := range manifest.Files {
		err := func() error {
			var r io.Reader
			var size int64
			if fe.Content != nil {
				r = bytes.NewReader(fe.Content)
				size = int64(len(fe.Content))
			} else {
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
				r = f
				size = fi.Size()
			}
			if err := writeFileToImage(imgBuf, fe.Path, size, r); err != nil {
				return err
			}
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
	_, err = io.Copy(imgFile, io.TeeReader(imgBuf, hasher))
	if err != nil {
		return err
	}

	hash = hex.EncodeToString(hasher.Sum(nil))
	if err = ioutil.WriteFile(imgFilename+".hash", []byte(hash), 0666); err != nil {
		return err
	}

	if manifest.NodeMCUFirmware != "" {
		binFilename := filepath.Join(outputDir, fmt.Sprintf("%s.bin", manifest.ID))
		hash, err = utils.CopyFile(manifest.NodeMCUFirmware, binFilename, true)
		if err != nil {
			return fmt.Errorf("Cannot copy NodeMCU firmware image %s to %s: %s", manifest.NodeMCUFirmware, outputDir, err)
		}
		err = ioutil.WriteFile(binFilename+".hash", []byte(hash), 0666)
	}

	return err
}

func Build(config *config.BuildConfig) error {
	if err := utils.RemoveDirContents(config.Output); err != nil {
		return fmt.Errorf("cannot remove output dir (%s) contents: %s", config.Output, err)
	}

	allLibs := make(map[string]*FirmwareLib)

	for _, libGlob := range config.Libs {
		libNames, _ := filepath.Glob(libGlob)
		for _, libName := range libNames {
			fi, err := os.Stat(libName)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				_, err = LoadLibrary(libName, allLibs, 0)
				if err != nil {
					return err
				}
			}
		}
	}

	for _, deviceDef := range config.Devices {
		devices, _ := filepath.Glob(deviceDef)
		for _, devicePath := range devices {
			fi, err := os.Stat(devicePath)
			if err != nil {
				return err
			}
			if fi.IsDir() {
				deviceRootLib, err := LoadLibrary(devicePath, allLibs, 0)
				if err != nil {
					return err
				}

				var fwDef FirmwareDef
				deviceName := filepath.Base(devicePath)
				if err := utils.ReadJSON(filepath.Join(devicePath, "firmware.json"), &fwDef); err != nil {
					return fmt.Errorf("Cannot read firmware file for %s in %s: %s", deviceName, devicePath, err)
				}

				manifest, err := buildDeviceFirmwareManifest(deviceRootLib, fwDef)
				if err != nil {
					return fmt.Errorf("Error building device firmware for device with name %q: %s", fi.Name(), err)
				}
				if err := utils.WriteJSON(filepath.Join(config.Output, manifest.ID+".json"), manifest); err != nil {
					return err
				}
				if err = writeFirmwareImage(manifest, config.Output); err != nil {
					return fmt.Errorf("Error writing firmware image for %s: %s", devicePath, err)
				}

			}
		}
	}
	return nil
}

func isLua(path string) bool {
	return filepath.Ext(path) == ".lua"
}
