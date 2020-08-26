// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"container/heap"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"istio.io/istio/tools/bug-report/pkg/logentry"
	"istio.io/pkg/log"
)

const (
	proxyLogsPath = "proxy-logs"
	istioLogsPath = "istio-logs"
)

var (
	// Each run of the command produces a new archive.
	instancePath = fmt.Sprint(rand.Int())
)

func CreateFromMap(logs map[string]*logentry.LogEntry, maxSize int) ([]byte, error) {
	var archiveBuffer bytes.Buffer
	gzw := gzip.NewWriter(&archiveBuffer)
	tw := tar.NewWriter(gzw)

	header := tar.Header{
		Typeflag: tar.TypeDir,
		Name: "logs/",
		Mode: 0655,
	}
	tw.WriteHeader(&header)


	// This builds a Max Heap based on priority and inverse the compressed file
	// size. The goal is to include as many high priority logs as possible in the
	// permitted file size.
	var logheap logentry.LogHeap
	heap.Init(&logheap)
	for _, val := range logs {
		newlog := val
		heap.Push(&logheap, newlog)
	}

	for logheap.Len() > 0 && len(archiveBuffer.Bytes()) < maxSize {
		val := heap.Pop(&logheap).(*logentry.LogEntry)
		log.Infof("Val: %v", val)
		for index, datum := range val.LogData {
			log.Infof("Datum: %v", datum)
			if len(datum) == 0 {
				continue
			}
			defer tw.Flush()
			defer gzw.Flush()

			header = tar.Header{
				Name: fmt.Sprintf("%s/%s.%d.log", strings.ToLower(val.Type), val.Path, index,),
				Size: int64(len(datum)),
				Mode: 0644,
			}
			if err := tw.WriteHeader(&header); err != nil {
				log.Errorf("Error adding header to tar archive: %s", err)
			}
			if _, err := tw.Write(datum); err != nil {
				log.Errorf("Error adding file data to tar archive: %s", err)
			}

			log.Infof("Wrote to archive: %s", val.Path)
		}


	}

	tw.Close()
	gzw.Close()
	return archiveBuffer.Bytes(), nil
}

// Create creates a gzipped tar file from srcDir and writes it to outPath.
func Create(srcDir, outPath string) error {
	mw, err := os.Create(outPath)
	if err != nil {
		return err
	}

	gzw := gzip.NewWriter(mw)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}
		header.Name = strings.TrimPrefix(strings.Replace(file, srcDir, "", -1), string(filepath.Separator))
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		f, err := os.Open(file)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		f.Close()

		return nil
	})
}
