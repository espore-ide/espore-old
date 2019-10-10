package builder

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"espore/utils"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gobwas/glob"
)

type DeviceInfo struct {
	Name string `json:"name"`
}

type FirmwareDef struct {
	DeviceInfo
	ID      string   `json:"id"`
	Libs    []string `json:"libs"`
	Modules []string `json:"modules"`
	LFS     []string `json:"lfs,omitempty"`
}

type FirmwareManifest struct {
	DeviceInfo
	Files map[string]string `json:"files"`
}

type FileEntry struct {
	fileName string
	hash     string
}

type Lib struct {
	files        map[string]string
	dependencies []string
}

func NewLib() *Lib {
	return &Lib{
		files: make(map[string]string),
	}
}

var parseDepRegex = regexp.MustCompile(`(?m)--\s*import:\s*(.*)\s*$`)

func parseDependencies(luaFile string, depMap map[string]bool) error {
	code, err := ioutil.ReadFile(luaFile)
	if err != nil {
		return err
	}
	matches := parseDepRegex.FindAllStringSubmatch(string(code), -1)
	if matches != nil {
		for _, match := range matches {
			depMap[match[1]] = true
		}
	}

	return nil
}

func packLib(libName, libPath string) (*Lib, error) {
	depMap := make(map[string]bool)
	files, err := ioutil.ReadDir(libPath)
	if err != nil {
		return nil, fmt.Errorf("Cannot read firmware folder %s: %s", libPath, err)
	}
	var lib Lib
	lib.files = make(map[string]string)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		filePath := filepath.Join(libPath, f.Name())
		hash, err := utils.HashFile(filePath)
		if err != nil {
			return nil, err
		}
		lib.files[fmt.Sprintf("/%s/%s", libName, f.Name())] = hash
		if filepath.Ext(filePath) == ".lua" {
			parseDependencies(filePath, depMap)
		}
	}
	utils.CopyDir(libPath, filepath.Join("dist", libName))
	for dep := range depMap {
		lib.dependencies = append(lib.dependencies, dep)
	}
	return &lib, nil
}

func packRecursive(libs map[string]Lib, baseName, basePath string) {
	libFolders, err := ioutil.ReadDir(basePath)
	if err != nil {
		log.Fatalf("Cannot find firmware folder: %s", err)
	}

	for _, lf := range libFolders {
		if lf.IsDir() {
			libPath := filepath.Join(basePath, lf.Name())
			libName := filepath.Join(baseName, lf.Name())
			lib, err := packLib(libName, libPath)
			if err != nil {
				log.Fatalf("Cannot pack lib %s: %s", libPath, err)
			}
			libs[libName] = *lib
			packRecursive(libs, libName, libPath)
		}
	}
}

func build() {
	libs := make(map[string]Lib)
	if err := utils.RemoveDirContents("dist"); err != nil {
		log.Fatalf("cannot remove dist dir contents: %s", err)
	}

	packRecursive(libs, "", "firmware")
	packRecursive(libs, "", filepath.Join("site", "lib"))

	deviceFolderName := filepath.Join("site", "devices")
	deviceFolders, err := ioutil.ReadDir(deviceFolderName)
	if err != nil {
		log.Fatalf("Cannot find devices folder: %s", err)
	}

	for _, df := range deviceFolders {
		if df.IsDir() {
			libPath := filepath.Join(deviceFolderName, df.Name())
			deviceLib, err := packLib(df.Name(), libPath)
			if err != nil {
				log.Fatalf("Cannot pack device lib %s: %s", libPath, err)
			}
			libs[df.Name()] = *deviceLib
			var fd FirmwareDef
			var fm FirmwareManifest
			fm.Files = make(map[string]string)
			if err := utils.ReadJSON(filepath.Join(libPath, "firmware.json"), &fd); err != nil {
				log.Fatal(err)
			}
			files := make(map[string]FileEntry)
			fm.DeviceInfo = fd.DeviceInfo
			if len(fd.LFS) > 0 {
				var lfsEntries []FileEntry
				for _, libName := range fd.LFS {
					lib, ok := libs[libName]
					if !ok {
						log.Fatalf("Cannot find lib %s defined in %s", libName, df.Name())
					}
					for fileName, hash := range lib.files {
						if filepath.Ext(fileName) == ".lua" {
							lfsEntries = append(lfsEntries, FileEntry{fileName, hash})
						} else {
							files[filepath.Base(fileName)] = FileEntry{fileName, hash}
						}
					}
				}
				sort.Slice(lfsEntries, func(i, j int) bool {
					return strings.Compare(lfsEntries[i].fileName, lfsEntries[j].fileName) < 0
				})

				var lfsFiles []string
				hasher := sha1.New()
				for _, entry := range lfsEntries {
					lfsFiles = append(lfsFiles, filepath.Join("dist", entry.fileName))
					hashBytes, _ := hex.DecodeString(entry.hash)
					hasher.Write(hashBytes)
				}
				contentHash := hex.EncodeToString(hasher.Sum(nil))

				imgFile := fmt.Sprintf("/%s-lfs.img", fd.ID)
				distImgFile := filepath.Join("dist", imgFile)
				cacheImgFile := filepath.Join("imgcache", imgFile+"."+contentHash)
				if _, err := os.Stat(cacheImgFile); os.IsNotExist(err) {
					os.MkdirAll(filepath.Join("imgcache"), 0775)
					err := utils.Luac(lfsFiles, cacheImgFile)
					if err != nil {
						log.Fatalf("Error compiling: %s", err)
					}
				}
				hash, err := utils.HashFile(cacheImgFile)
				if err != nil {
					log.Fatalf("Error hashing lfs image: %s", err)
				}
				utils.CopyFile(cacheImgFile, distImgFile)
				files[filepath.Base(imgFile)] = FileEntry{imgFile, hash}
			}

			processedLibs := make(map[string]bool)
			var allLibs []string
			for _, libName := range fd.Libs {
				if processedLibs[libName] {
					continue
				}
				lib, ok := libs[libName]
				if !ok {
					log.Fatalf("Cannot find lib %s defined in %s", libName, df.Name())
				}
				for _, dep := range lib.dependencies {
					if processedLibs[dep] {
						continue
					}
					processedLibs[dep] = true
					allLibs = append(allLibs, dep)
				}
				processedLibs[libName] = true
				allLibs = append(allLibs, libName)
			}

			for _, libName := range allLibs {
				lib, ok := libs[libName]
				if !ok {
					log.Fatalf("Cannot find lib %s defined in %s", libName, df.Name())
				}
				for fileName, hash := range lib.files {
					files[filepath.Base(fileName)] = FileEntry{fileName, hash}
				}
			}
			for fileName, hash := range deviceLib.files {
				files[filepath.Base(fileName)] = FileEntry{fileName, hash}
			}

			for _, entry := range files {
				fm.Files[entry.fileName] = entry.hash
			}
			if err := utils.WriteJSON(filepath.Join("dist", fmt.Sprintf("%s.json", fd.ID)), fm); err != nil {
				log.Fatalf("Error writing firmware manifest for device %s: %s", df.Name(), err)
			}
		}
	}
}

type FirmwareRoot struct {
	BasePath string
	Files    map[string]*FileEntry2
}

type FileEntry2 struct {
	Base         string   `json:"base"`
	Path         string   `json:"path"`
	Hash         string   `json:"hash"`
	Dependencies []string `json:"-"`
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

type FirmwareDef2 struct {
	DeviceInfo
	ID      string      `json:"id"`
	Libs    []LibDef    `json:"libs"`
	Modules []ModuleDef `json:"modules"`
}

type FirmwareManifest2 struct {
	DeviceInfo
	Files []*FileEntry2 `json:"files"`
}

var parseDepRegex2 = regexp.MustCompile(`(?m)require\s*\(\s*"([^"]*)"\s*\)`)

func ReadDependencies(luaFile string) ([]string, error) {
	code, err := ioutil.ReadFile(luaFile)
	if err != nil {
		return nil, err
	}
	depMap := make(map[string]bool)
	matches := parseDepRegex2.FindAllStringSubmatch(string(code), -1)
	if matches != nil {
		for _, match := range matches {
			depMap[match[1]] = true
		}
	}
	var deps []string
	for dep := range depMap {
		deps = append(deps, dep)
	}

	return deps, nil
}

func AddRoot(path string, roots map[string]FirmwareRoot) error {
	list, err := utils.CopyAndEnumerateDir(path, filepath.Join("dist", path))
	if err != nil {
		return err
	}
	entries := make(map[string]*FileEntry2)
	for _, f := range list {
		var entry FileEntry2
		fpath := filepath.Join(path, f)
		entry.Path = f
		entry.Base = path
		entry.Hash, err = utils.HashFile(fpath)
		if err != nil {
			return err
		}
		if filepath.Ext(f) == ".lua" {
			deps, err := ReadDependencies(fpath)
			if err != nil {
				return err
			}
			entry.Dependencies = deps
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

func FindInRoots(fileName string, roots []FirmwareRoot) (*FileEntry2, error) {
	for _, root := range roots {
		entry, ok := root.Files[fileName]
		if ok {
			return entry, nil
		}
	}
	return nil, ErrFileEntryNotFound
}

func AddFilesFromModule(moduleName string, roots []FirmwareRoot, fileMap map[string]*FileEntry2) error {
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

func AddOtherFiles(allRoots map[string]FirmwareRoot, libs []LibDef, fileMap map[string]*FileEntry2) error {
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

func buildDeviceFirmware(allRoots map[string]FirmwareRoot, deviceName string) error {
	var fwDef FirmwareDef2
	devicePath := filepath.Join("site/devices", deviceName)
	if err := utils.ReadJSON(filepath.Join(devicePath, "firmware.json"), &fwDef); err != nil {
		return fmt.Errorf("Cannot read firmware file for %s: %s", deviceName, err)
	}
	roots, err := getDeviceFirmwareRoots(allRoots, fwDef.Libs)
	if err != nil {
		return fmt.Errorf("Cannot build firmware roots for %s: %s", deviceName, err)
	}

	fileMap := make(map[string]*FileEntry2)
	for _, modDef := range fwDef.Modules {
		if err := AddFilesFromModule(modDef.Name, roots, fileMap); err != nil {
			return fmt.Errorf("Cannot add files from module %s: %s", modDef.Name, err)
		}
	}

	if err := AddOtherFiles(allRoots, fwDef.Libs, fileMap); err != nil {
		return fmt.Errorf("Error adding other files in device %s: %s", deviceName, err)
	}

	var manifest FirmwareManifest2
	manifest.DeviceInfo = fwDef.DeviceInfo
	manifest.Name = fwDef.Name
	manifest.Files = make([]*FileEntry2, 0, len(fileMap))
	for _, file := range fileMap {
		manifest.Files = append(manifest.Files, file)
	}

	return utils.WriteJSON(filepath.Join("dist", fwDef.ID+".json"), manifest)
}

func Build2() error {
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
			err := buildDeviceFirmware(roots, fd.Name())
			if err != nil {
				return err
			}
		}
	}

	return nil
}
