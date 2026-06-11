package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCompletionsCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completions [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for orbiter.

Add completions to your shell:

  bash:        source <(orbiter completions bash)
  zsh:         source <(orbiter completions zsh)
  fish:        orbiter completions fish | source
  powershell:  orbiter completions powershell | Out-String | Invoke-Expression`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		// Override parent PersistentPreRunE — completions needs no star chart.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletion(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	return cmd
}
