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

package bugreport

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"istio.io/istio/operator/pkg/util"
	"istio.io/istio/tools/bug-report/pkg/archive"
	cluster2 "istio.io/istio/tools/bug-report/pkg/cluster"
	"istio.io/istio/tools/bug-report/pkg/config"
	"istio.io/istio/tools/bug-report/pkg/content"
	"istio.io/istio/tools/bug-report/pkg/filter"
	"istio.io/istio/tools/bug-report/pkg/kubeclient"
	"istio.io/istio/tools/bug-report/pkg/kubectlcmd"
	"istio.io/istio/tools/bug-report/pkg/logentry"
	"istio.io/istio/tools/bug-report/pkg/processlog"
	"istio.io/pkg/log"
	"istio.io/pkg/version"
)

const (
	bugReportDefaultMaxSizeMb = 500
	bugReportDefaultTimeout   = 30 * time.Minute
	bugReportDefaultTempDir   = "/tmp/bug-report"
)

var (
	bugReportDefaultOutputDir = ""
	bugReportDefaultIstioNamespaces = []string{"istio-system"}
	bugReportDefaultInclude         = []string{""}
	bugReportDefaultExclude         = []string{"kube-system,kube-public"}
)

// GetRootCmd returns the root of the cobra command-tree.
func GetRootCmd(args []string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "bug-report",
		Short:        "Cluster information and log capture support tool.",
		SilenceUsage: true,
		Long: "This command selectively captures cluster information and logs into an archive to help " +
			"diagnose problems. It optionally uploads the archive to a GCS bucket.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBugReportCommand(cmd)
		},
	}
	rootCmd.SetArgs(args)
	rootCmd.AddCommand(version.CobraCommand())
	addFlags(rootCmd, gConfig)

	return rootCmd
}

func runBugReportCommand(cmd *cobra.Command) error {
	config, err := parseConfig()
	if err != nil {
		return err
	}

	_, clientset, err := kubeclient.InitK8SRestClient(config.KubeConfigPath, config.Context)
	if err != nil {
		return fmt.Errorf("could not initialize k8s client: %s ", err)
	}
	resources, err := cluster2.GetClusterResources(context.Background(), clientset)
	if err != nil {
		return err
	}

	log.Infof("Cluster resource tree:\n\n%s\n\n", resources)
	paths, err := filter.GetMatchingPaths(config, resources)
	if err != nil {
		return err
	}

	var errs util.Errors
	logs := make(map[string]*logentry.LogEntry)
	lock := sync.RWMutex{}
	var wg sync.WaitGroup
	for _, p := range paths {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			var wg2 sync.WaitGroup
			cv := strings.Split(p, ".")
			namespace, pod, container := cv[0], cv[2], cv[3]

			wg.Add(1)
			go func() {
				defer wg.Done()
				clog, cstat, imp, err := getLog(resources, config, namespace, pod, container)
				lock.Lock()
				logs[p] = logentry.NewContainerLog(p, clog, 0, cstat, imp, err)
				lock.Unlock()
			}()

			if container == "istio-proxy" {
				wg.Add(1)
				go func() {
					defer wg2.Done()
					cds, err := content.GetCoredumps(namespace, pod, container, config.DryRun)
					lock.Lock()
					logs[p] = logentry.NewCoreDumpLog(p, err)
					for index, coredump := range cds {
						logs[p].SetLogString(coredump, index)
					}
					lock.Unlock()
				}()
			}

			if strings.HasPrefix(pod, "istiod-") {
				wg.Add(1)
				go func() {
					defer wg2.Done()
					info, err := content.GetIstiodInfo(namespace, pod, config.DryRun)
					lock.Lock()
					errs = util.AppendErr(errs, err)
					lock.Unlock()
					fmt.Println(info)
				}()
			}
		}()
	}
	wg.Wait()

	log.Infof("Outputting archive:")
	ar, err := archive.CreateFromMap(logs, 40)
	if err != nil {
		log.Errorf("Error creating archive from map: %s", err)
	}
	log.Infof("Archive: %v", ar)
	ioutil.WriteFile("./test.tgz", ar, 0644)

	return errs.ToError()
}

func getLog(resources *cluster2.Resources, config *config.BugReportConfig, namespace, pod, container string) (string, *processlog.Stats, int, error) {
	log.Infof("Getting logs for %s/%s/%s...", namespace, pod, container)
	previous := resources.ContainerRestarts(pod, container) > 0
	clog, err := kubectlcmd.Logs(namespace, pod, container, previous, config.DryRun)
	if err != nil {
		return "", nil, 0, err
	}
	cstat := &processlog.Stats{}
	clog, cstat, err = processlog.Process(config, clog)
	if err != nil {
		return "", nil, 0, err
	}
	return clog, cstat, cstat.Importance(), nil
}
