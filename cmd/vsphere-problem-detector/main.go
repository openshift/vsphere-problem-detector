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

func main() {
	pflag.CommandLine.SetNormalizeFunc(k8sflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	logs.InitLogs()
	defer logs.FlushLogs()

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

	versionCmd := &cobra.Command{
		Use:	"version",
		Short:  "Print the version number of vSphere Problem Detector",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Get())
		},
	}

	cmd.AddCommand(ctrlCmd)
	cmd.AddCommand(versionCmd)

	return cmd
}
