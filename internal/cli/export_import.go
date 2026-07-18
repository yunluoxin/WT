package cli

import (
	"github.com/spf13/cobra"

	"wt/internal/ops"
)

func exportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export worktree configuration and metadata to a file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ExportConfig(output)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: wt-export-<timestamp>.json)")
	return cmd
}

func importCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import worktree configuration and metadata from a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ImportConfig(args[0], apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply the import (default: preview only)")
	return cmd
}
