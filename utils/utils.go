package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

func CopyFile(src, dst string, hashFile bool) (h string, err error) {
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo
	var hasher hash.Hash
	var r io.Reader
	if hashFile {
		hasher = sha1.New()
	}

	if srcfd, err = os.Open(src); err != nil {
		return "", err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return "", err
	}

	if hashFile {
		r = io.TeeReader(srcfd, hasher)
	} else {
		r = srcfd
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, r); err != nil {
		return "", err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return "", err
	}
	if hashFile {
		h = hex.EncodeToString(hasher.Sum(nil))
	}
	return h, os.Chmod(dst, srcinfo.Mode())
}

func enumerateDir(basePath, src string, fileList []string) ([]string, error) {
	var err error
	var fds []os.FileInfo

	if _, err = os.Stat(src); err != nil {
		return fileList, err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return fileList, err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())

		if fd.IsDir() {
			if fileList, err = enumerateDir(path.Join(basePath, fd.Name()), srcfp, fileList); err != nil {
				return fileList, err
			}
		} else {
			fileList = append(fileList, path.Join(basePath, fd.Name()))
		}
	}
	return fileList, nil
}

func EnumerateDir(src string) ([]string, error) {
	return enumerateDir("", src, nil)
}

func copyDir(basePath, src, dst string, fileList []string) ([]string, error) {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return fileList, err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return fileList, err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return fileList, err
	}
	for _, fd := range fds {
		srcfp := path.Join(src, fd.Name())
		dstfp := path.Join(dst, fd.Name())

		if fd.IsDir() {
			if fileList, err = copyDir(path.Join(basePath, fd.Name()), srcfp, dstfp, fileList); err != nil {
				return fileList, err
			}
		} else {
			if _, err = CopyFile(srcfp, dstfp, false); err != nil {
				return fileList, err
			}
			fileList = append(fileList, path.Join(basePath, fd.Name()))
		}
	}
	return fileList, nil
}

func CopyDir(src string, dst string) error {
	_, err := copyDir("", src, dst, nil)
	return err
}

func CopyAndEnumerateDir(src string, dst string) ([]string, error) {
	return copyDir("", src, dst, nil)
}

func ReadJSON(path string, item interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, item)
}

func WriteJSON(path string, item interface{}) error {
	data, err := json.MarshalIndent(item, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, data, 0666)
}

func RemoveDirContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

func HashFile(path string) (string, error) {
	hasher := sha1.New()
	s, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	_, err = hasher.Write(s)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
