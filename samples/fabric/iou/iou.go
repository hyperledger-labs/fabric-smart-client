package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/topology"
)

const (
	ProgramName = "iou"
)

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{Use: "iou"}

func main() {
	// For environment variables.
	viper.SetEnvPrefix(ProgramName)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// Define command-line flags that are valid for all peer commands and
	// subcommands.
	mainFlags := mainCmd.PersistentFlags()

	mainFlags.String("logging-level", "", "Legacy logging level flag")
	viper.BindPFlag("logging_level", mainFlags.Lookup("logging-level"))
	mainFlags.MarkHidden("logging-level")

	mainCmd.AddCommand(GenerateCmd())
	mainCmd.AddCommand(CleanCmd())
	mainCmd.AddCommand(StartCmd())
	mainCmd.AddCommand(VersionCmd())

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

// VersionCmd returns the Cobra Command for Version
func VersionCmd() *cobra.Command {
	return versionCommand
}

var versionCommand = &cobra.Command{
	Use:   "version",
	Short: "Print iou version.",
	Long:  `Print current version of IOU CLI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		fmt.Print(Version())
		return nil
	},
}

// Version returns version information for the peer
func Version() string {
	return fmt.Sprintf("%s:\n Go version: %s\n"+
		" OS/Arch: %s\n",
		ProgramName, runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
}

// GenerateCmd returns the Cobra Command for Generate
func GenerateCmd() *cobra.Command {
	return GenerateCommand
}

var GenerateCommand = &cobra.Command{
	Use:   "generate",
	Short: "Generate Artifacts.",
	Long:  `Generate Artifacts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return Generate()
	},
}

// Generate returns version information for the peer
func Generate() error {
	_, err := integration.GenerateAt(20000, "./artifacts", true, topology.Topology()...)
	return err
}

// CleanCmd returns the Cobra Command for Clean
func CleanCmd() *cobra.Command {
	return CleanCommand
}

var CleanCommand = &cobra.Command{
	Use:   "clean",
	Short: "Clean Artifacts.",
	Long:  `Clean Artifacts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return Clean()
	},
}

// Clean returns version information for the peer
func Clean() error {
	// delete artifacts folder
	err := os.RemoveAll("./artifacts")
	if err != nil {
		return err
	}
	// delete cmd folder
	err = os.RemoveAll("./cmd")
	if err != nil {
		return err
	}
	return nil
}

// StartCmd returns the Cobra Command for Start
func StartCmd() *cobra.Command {
	return StartCommand
}

var StartCommand = &cobra.Command{
	Use:   "start",
	Short: "Start Artifacts.",
	Long:  `Start Artifacts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return Start()
	},
}

// Start returns version information for the peer
func Start() error {
	ii, err := integration.Load("./artifacts", true, iou.Topology()...)
	if err != nil {
		return err
	}
	ii.DeleteOnStop = false
	ii.Start()

	ii.Stop()
	return nil
}
