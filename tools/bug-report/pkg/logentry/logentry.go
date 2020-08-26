package logentry

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"istio.io/pkg/log"

	"istio.io/istio/tools/bug-report/pkg/processlog"
)

// This is a type to encapsulate a single LogEntry so it can be handled
// the same way regardless of what *type* of Log it is.
type LogEntry struct {
	// Path of the log source, informational
	Path string
	// Type of log, informational
	Type string
	LogCaptureStats *processlog.Stats
	LogImportance int
	CompressedSize int
	LogData [][]byte
	Error error

}

func NewLogEntry(logtype string) *LogEntry {
	le := LogEntry{Type: logtype}
	le.LogData = make([][]byte, 1)
	return &le
}

func NewContainerLog(path string, clog string, index int, cstat *processlog.Stats, logimportance int, err error) *LogEntry {
	le := NewLogEntry("Container")
	le.Path = path
	le.LogCaptureStats = cstat
	le.LogImportance = logimportance
	le.Error = err
	le.SetLogString(clog, index)
	return le
}

func NewCoreDumpLog(path string, err error) *LogEntry {
	le := NewLogEntry("CoreDump")
	le.Path = path
	return le
}

func (le *LogEntry) SetLogString(in string, index int) (error) {
	log.Infof("Path: %s, Type: %s, Log: %s", le.Path, le.Type, in)
	return le.SetLogBytes([]byte(in), index)
}

func (le *LogEntry) SetLogBytes(in []byte, index int) (error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)

	_, err := gzw.Write(in)
	if err != nil {
		log.Fatalf("%s", err)
	}
	// Expand and init the logdata struct
	if le.LogData[index] == nil {
		le.LogData[index] = make([]byte, 1)
	}
	le.LogData[index] = buf.Bytes()
	gzw.Close()
	le.CompressedSize = 0
	for _, logDatum := range le.LogData {
		le.CompressedSize += len(logDatum)
	}
	return nil
}

func (le *LogEntry) updateCompressedSize() {
	var val int = 0

	le.CompressedSize = val
}

func (le *LogEntry) GetLogString(index int) (string, error) {
	logbyte, err := le.GetLogBytes(index)
	if err != nil {
		log.Fatalf("%s", err)
	}
	return string(logbyte), err
}

func (le *LogEntry) GetLogBytes(index int) ([]byte, error) {
	data := bytes.NewBuffer(le.LogData[index])
	gzr, err := gzip.NewReader(data)
	defer gzr.Close()
	if err != nil {
		log.Fatalf("Cannot get LogBytes for %s at index %d: %s", le.Path, index, err)
	}
	return ioutil.ReadAll(gzr)
}

func (le *LogEntry) GetLenBytes(index int) (int) {
	b, _ := le.GetLogBytes(index)
	return len(b)
}