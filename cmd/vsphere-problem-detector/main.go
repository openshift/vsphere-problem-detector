package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/component-base/cli"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/vsphere-problem-detector/pkg/operator"
	"github.com/openshift/vsphere-problem-detector/pkg/version"
	"k8s.io/utils/clock"
)

func main() {
	command := NewOperatorCommand()
	code := cli.Run(command)
	os.Exit(code)
}

func NewOperatorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vsphere-problem-detector",
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
		clock.RealClock{},
	).NewCommand()
	ctrlCmd.Use = "start"
	ctrlCmd.Short = "Start the vSphere Problem Detector"

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of vSphere Problem Detector",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Get())
		},
	}

	cmd.AddCommand(ctrlCmd)
	cmd.AddCommand(versionCmd)

	return cmd
}
