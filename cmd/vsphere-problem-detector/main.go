package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	k8sflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/vsphere-problem-detector/pkg/operator"
	"github.com/openshift/vsphere-problem-detector/pkg/version"
)

var (
	runInVanillaKube = pflag.Bool("vanilla-kube", false, "Run in vanilla kube (default: false)")
	cloudConfig      = pflag.String("cloud-config", "", "Location of vsphere cloud-configuration")
)

func main() {
	pflag.CommandLine.SetNormalizeFunc(k8sflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()
	pflag.Parse()

	if cloudConfig != nil {
		operator.CloudConfigLocation = *cloudConfig
	}
	if runInVanillaKube != nil {
		operator.RunningInVanillaKube = *runInVanillaKube
	}

	command := NewOperatorCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vsphere-monitoring-operator",
		Short: "OpenShift vSphere Problem Detector",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
			os.Exit(1)
		},
	}

	ctrlCmd := controllercmd.NewControllerCommandConfig(
		"vsphere-problem-detector",
		version.Get(),
		operator.RunOperator,
	).NewCommand()
	ctrlCmd.Use = "start"
	ctrlCmd.Short = "Start the vSphere Problem Detector"

	cmd.AddCommand(ctrlCmd)

	return cmd
}
