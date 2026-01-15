package commands

import (
	"os"
	"path/filepath"

	"github.com/LumeraProtocol/sdk-go/cascade"
	"github.com/LumeraProtocol/sdk-go/types"
	"github.com/spf13/cobra"

	"lumera-ica-client/client"
)

// newDownloadCmd registers the "download" command and streams artefacts from supernodes.
// It signs the download request with the controller owner address and returns the output path.
func newDownloadCmd(app *app) *cobra.Command {
	var actionID string
	var outDir string
	cmd := &cobra.Command{
		Use:   "download [action-id]",
		Short: "Download file by action ID",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			// Resolve input, load config, and start a bounded command context.
			actionID, err = resolveOptionalArg(actionID, args, "action-id")
			if err != nil {
				return err
			}
			cfg, err := app.loadConfig()
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd)
			defer cancel()

			// Ensure the output directory exists before invoking the SDK.
			outDir = filepath.Clean(outDir)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			// Initialize cascade client + controller for ICA signature generation.
			cascClient, err := client.NewCascadeClient(ctx, cfg)
			if err != nil {
				return err
			}
			defer cascClient.Cascade.Close()

			controller, err := client.NewICAController(ctx, cfg, cascClient.Keyring)
			if err != nil {
				return err
			}
			defer controller.Close()
			// Start the download; the SDK handles task creation and wait.
			res, err := cascClient.Cascade.Download(ctx, actionID, outDir, cascade.WithDownloadSignerAddress(controller.OwnerAddress()))
			if err != nil {
				return err
			}
			// Best-effort lookup for the original filename from the action metadata.
			fileName := ""
			if bc, err := client.NewLumeraClient(ctx, cfg, cascClient.Keyring, cfg.Controller.KeyName); err == nil {
				defer bc.Close()
				if action, err := bc.Action.GetAction(ctx, actionID); err == nil {
					if meta, ok := action.Metadata.(*types.CascadeMetadata); ok && meta != nil {
						fileName = meta.FileName
					}
				}
			}
			return writeJSON(map[string]any{
				"status":      "ok",
				"action_id":   res.ActionID,
				"task_id":     res.TaskID,
				"output_path": res.OutputPath,
				"file_name":   fileName,
			})
		},
	}
	cmd.Flags().StringVar(&actionID, "action-id", "", "Action ID to download")
	cmd.Flags().StringVar(&outDir, "out", ".", "Output directory")
	return cmd
}
