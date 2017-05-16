/*
Copyright 2017 Mirantis

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hpcloud/tail"
)

type VirtletLoggerConf struct {
	VirtletFolder      string
	VirtletFilename    string
	KubernetesFolder   string
	KubernetesFilename string
}

type VirtletLogger interface {
	SpawnWorkers() error
	Worker(inputFile, outputFile, sandboxId string)
	StopObsoleteWorkers()
	StopAllWorkers()
}

func NewVirtletLogger(virtletFolder, kubernetesFolder string) VirtletLogger {
	return &virtletLogger{
		Config: VirtletLoggerConf{
			VirtletFolder:      virtletFolder,
			VirtletFilename:    "raw.log",
			KubernetesFolder:   kubernetesFolder,
			KubernetesFilename: "_0.log",
		},
	}
}

type virtletLogger struct {
	Config  VirtletLoggerConf
	workers map[string]chan *tail.Line // map[sandboxId]chan
}

func (v *virtletLogger) SpawnWorkers() error {
	fmt.Println("Check and spawn workers")

	if v.workers == nil {
		v.workers = make(map[string]chan *tail.Line)
	}

	vmFolders, err := ioutil.ReadDir(v.Config.VirtletFolder)
	if err != nil {
		return err
	}

	for _, vmFolder := range vmFolders {
		sandboxId := vmFolder.Name()

		if v.workers[sandboxId] != nil {
			fmt.Printf("worker for sandbox '%s' already running. Skip.\n", sandboxId)
			continue
		}

		inputFile := filepath.Join(v.Config.VirtletFolder, sandboxId, v.Config.VirtletFilename)
		outputFile := filepath.Join(v.Config.KubernetesFolder, sandboxId, v.Config.KubernetesFilename)
		if !vmFolder.IsDir() {
			continue
		}
		if _, err := os.Stat(inputFile); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(filepath.Dir(outputFile)); os.IsNotExist(err) {
			continue
		}

		go v.Worker(inputFile, outputFile, sandboxId)
	}

	return nil
}

func (v *virtletLogger) Worker(inputFile, outputFile, sandboxId string) {
	fmt.Println("Spawned worker for:", sandboxId)

	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		file, err := os.Create(outputFile)
		if err != nil {
			fmt.Println("failed to create output file:", err)
			return
		}
		file.Close()
	}

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0777)
	if err != nil {
		fmt.Println("failed to open output file:", err)
		return
	}
	defer f.Close()

	// Tail VM's file.
	t, err := tail.TailFile(inputFile, tail.Config{Follow: true})
	if err != nil {
		fmt.Println("failed to tail input file:", err)
		return
	}

	// Expose channel to virtletLogger so that it can close it when needed.
	v.workers[sandboxId] = t.Lines

	// Do work. This forloop will block until canceled; it will wait for new
	// lines to come and parse them immediately.
	for line := range t.Lines {
		// Convert raw line into Kubernetes json.
		converted := fmt.Sprintf(`{"time": "%s", "stream": "stdout","log":"%s\n"}`, line.Time.Format(time.RFC3339), escapeLine(line.Text))
		converted = converted + "\n"

		f.WriteString(converted)
		f.Sync()
	}

	// This code is only executed when t.Lines channel is closed.
	delete(v.workers, sandboxId)
	fmt.Println("Worker stopped gracefully")
}

func (v *virtletLogger) StopObsoleteWorkers() {
	fmt.Println("Stop obsolete workers")
	for sandboxId, _ := range v.workers {
		if _, err := os.Stat(filepath.Join(v.Config.KubernetesFolder, sandboxId)); os.IsNotExist(err) {
			v.stopWorker(sandboxId)
		}
	}
}

func (v *virtletLogger) StopAllWorkers() {
	fmt.Println("Stop all workers")
	for sandboxId, _ := range v.workers {
		v.stopWorker(sandboxId)
	}

	for {
		if len(v.workers) == 0 {
			return
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func (v *virtletLogger) stopWorker(sandboxId string) {
	if v.workers[sandboxId] != nil {
		fmt.Println("stop", sandboxId)
		close(v.workers[sandboxId])
	}
}

func escapeLine(line string) string {
	line = strings.TrimRightFunc(line, unicode.IsSpace)
	line = strings.Replace(line, "\"", "\\\"", -1)
	return line
}
