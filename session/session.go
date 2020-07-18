package session

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"espore/session/bufferedwriter"
	"espore/session/fileman"
	"espore/session/lockreader"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const throttle = 100 * time.Millisecond
const chunkSize = 128

type Logger interface {
	Printf(fmt string, item ...interface{})
}
type Config struct {
	Socket io.ReadWriteCloser
	Output io.Writer
}

type Session struct {
	*bufferedwriter.BufferedWriter
	*lockreader.LockReader
	Log  Logger
	File *fileman.Fileman
}

type defaultLogger struct{}

func (dl *defaultLogger) Printf(fmt string, item ...interface{}) {
	log.Printf(fmt, item...)
}

func New(config *Config) (*Session, error) {
	s := &Session{
		Log: &defaultLogger{},
	}
	s.BufferedWriter = bufferedwriter.New(config.Socket)
	s.LockReader = lockreader.New(config.Socket)
	s.File = fileman.New(s)

	return s, nil
}

func (s *Session) SendCommand(cmd string) error {
	sw := NewLineWriter(s)
	_, err := sw.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

func (s *Session) pushRuntime(socket io.Reader) error {
	var err error
	defer func() {
		if err == nil {
			s.Log.Printf("OK\n")
		} else {
			s.Log.Printf("ERROR\n")
		}
	}()

	s.Log.Printf("Activating espore ...")

	if err = s.SendCommand("\nrequire('__espore')\n"); err != nil {
		return err
	}

	var r []string
	if r, err = awaitRegex(socket, `(READY|module '__espore' not found:)$`); err != nil {
		return errors.New("Pushing runtime failed")
	}

	if r[1] != "READY" {
		s.SendCommand("f = file.open('__espore.lua', 'w+')")
		lines := strings.Split(EsporeLua, "\n")
		for _, line := range lines {
			s.SendCommand(fmt.Sprintf("f:write([[%s]] .. '\\n')", line))
		}
		s.SendCommand("f:close()\nf=nil")

		if err = s.SendCommand("\nrequire('__espore')\n"); err != nil {
			return err
		}

		if r, err = awaitRegex(socket, `(READY|module '__espore' not found:)$`); err != nil {
			return errors.New("Pushing runtime failed")
		}
		if r[1] != "READY" {
			return errors.New("Error uploading espore runtime")
		}
	}

	return nil
}

func (s *Session) InstallRuntime() error {
	return s.PushStream(bytes.NewBufferString(EsporeLua), int64(len(EsporeLua)), "__espore.lua")
}

func (s *Session) startUpload(fname string, size int64) error {
	if err := s.SendCommand(fmt.Sprintf("__espore.upload(\"%s\", %d)\n", fname, size)); err != nil {
		return err
	}
	return nil
}

func (s *Session) NodeRestart() error {
	return s.RunCode("node.restart()")
}

func (s *Session) PushStream(reader io.Reader, size int64, dstName string) error {
	const tmpfile = "__upload.tmp"
	err := s.LockReader.Lock(func(socket io.Reader) error {
		if err := s.ensureRuntime(socket); err != nil {
			return err
		}
		s.Log.Printf("Pushing %s ", dstName)
		sw := NewSlowWriter(s)

		if err := s.startUpload(tmpfile, size); err != nil {
			return err
		}

		if _, err := awaitRegex(socket, "BEGIN"); err != nil {
			return errors.New("Error waiting for upload BEGIN signal")
		}

		wg := new(sync.WaitGroup)
		wg.Add(2)
		var copyErr error
		var recvErr error
		var hash string
		var progressCount int64
		rc := make(chan int64)

		go func() {
			hasher := sha1.New()
			reader := io.TeeReader(reader, hasher)
			sent := int64(0)
			buf := make([]byte, chunkSize)
			defer func() {
				hash = hex.EncodeToString(hasher.Sum(nil))
				wg.Done()
			}()

			for {
				received, ok := <-rc
				if !ok {
					return
				}
				if received > progressCount {
					s.Log.Printf(".")
					progressCount += size / 10
				}
				if sent-received == 0 {
					i, err := reader.Read(buf)
					if i > 0 {
						_, copyErr = sw.Write(buf[:i])
						if copyErr != nil {
							return
						}
						sent += int64(i)
					}
					if err != nil {
						if err != io.EOF {
							copyErr = err
						}
						return
					}
				}
			}
		}()
		go func() {
			defer wg.Done()
			defer close(rc)
			var received = int64(0)
			for received < size {
				rc <- received
				st, err := awaitRegex(socket, `(\d+)$`)
				if err != nil {
					recvErr = fmt.Errorf("Error waiting for download progress response: %s", err)
					return
				}
				received, err = strconv.ParseInt(st[1], 10, 64)
				if err != nil {
					recvErr = fmt.Errorf("Error parsing remaining size: %s", err)
					return
				}
			}
		}()
		wg.Wait()
		if copyErr != nil {
			return fmt.Errorf("Error pushing file: %s", copyErr)
		}
		if recvErr != nil {
			return fmt.Errorf("Error receiving file: %s", recvErr)
		}
		m, err := awaitRegex(socket, "([0-9a-fA-F]{40})")
		if err != nil {
			return errors.New("Error waiting for file checksum hash")
		}
		if m[1] != hash {
			return fmt.Errorf("Checksum hash mismatch. Expected %s, got %s", hash, m[1])
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.File.Rename(tmpfile, dstName); err != nil {
		s.Log.Printf("ERROR\n")
		return err
	}
	s.Log.Printf("OK\n")
	return nil
}

func (s *Session) PushFile(srcPath, dstName string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	return s.PushStream(file, info.Size(), dstName)
}

type RPCResponse struct {
	RetVal json.RawMessage `json:"ret"`
	Err    string          `json:"err,omitempty"`
}

func (s *Session) Rpc(luaCode string) ([]byte, error) {
	var result []byte
	err := s.LockReader.Lock(func(socket io.Reader) error {
		if err := s.ensureRuntime(socket); err != nil {
			return err
		}
		template := "__espore.call(function()\n%s\nend)"
		s.RunCode(fmt.Sprintf(template, luaCode))
		r, err := AwaitStjson(socket)
		if err != nil {
			return errors.New("Error receiving RPC response")
		}
		jsonBytes := []byte(r)
		var response RPCResponse
		err = json.Unmarshal(jsonBytes, &response)
		if err != nil {
			return errors.New("Error decoding RPC response")
		}
		if response.Err != "" {
			return fmt.Errorf("RPC Error: %s", response.Err)
		}
		result = response.RetVal
		return nil
	})
	return result, err
}

func (s *Session) Close() error {
	defer s.BufferedWriter.Close()
	return s.SendCommand("\n__espore.finish()\n")
}

func (s *Session) GetChipID() (string, error) {
	var result string
	err := s.LockReader.Lock(func(reader io.Reader) error {
		if err := s.SendCommand("\nprint('i' .. 'd=' .. node.chipid())\n"); err != nil {
			return err
		}

		match, err := awaitRegex(reader, "id=(.*)")
		if err != nil {
			return err
		}
		result = match[1]
		return nil
	})
	return result, err
}

func (s *Session) ensureRuntime(reader io.Reader) error {
	err := s.SendCommand("\nprint(\"espore=\" .. tostring(__espore ~= nil))\n")
	if err != nil {
		return err
	}
	installedStr, err := awaitRegex(reader, "espore=(true|false)$")
	if err != nil {
		return errors.New("Error ensuring __espore is installed")
	}
	if installedStr[1] == "true" {
		return nil
	}
	return s.pushRuntime(reader)
}

func (s *Session) RunCode(luaCode string) error {
	return s.SendCommand(fmt.Sprintf(`
(function ()
%s
end)()
`, luaCode))

}

func ReadLine(r io.Reader) (string, error) {
	b := make([]byte, 1)
	buf := make([]byte, 0, 1024)
	for {
		i, err := r.Read(b)
		if i > 0 {
			if b[0] == 13 {
				continue
			}
			if b[0] == 10 {
				return string(buf), nil
			}
			buf = append(buf, b[0])
		}
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return string(buf), err
		}
	}
}

func awaitRegex(reader io.Reader, regexSt string) ([]string, error) {
	timeout := time.After(time.Second * 10)
	r := regexp.MustCompile(regexSt)

	for {
		line, err := ReadLine(reader)
		if err != nil {
			break
		}
		match := r.FindStringSubmatch(line)
		if len(match) > 0 {
			return match, nil
		}
		select {
		case <-timeout:
			return nil, errors.New("regex not found")
		default:

		}
	}
	return nil, errors.New("regex not found")
}

func AwaitStjson(reader io.Reader) (string, error) {
	timeout := time.After(time.Second * 10)
	openBrackets := 0
	started := false

	sb := strings.Builder{}
	for {
		line, err := ReadLine(reader)
		if err != nil {
			return "", err
		}
		switch line {
		case "{":
			openBrackets++
			started = true
		case "}":
			openBrackets--
			fallthrough
		case ",":
			timeout = time.After(time.Second * 10)
		}
		if started {
			sb.WriteString(line)
			sb.WriteByte(10)
			if openBrackets == 0 {
				return sb.String(), nil
			}
		}
		select {
		case <-timeout:
			return "", errors.New("regex not found")
		default:
		}
	}
}
