package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	actiontypes "github.com/LumeraProtocol/lumera/x/action/v1/types"
	"github.com/LumeraProtocol/sdk-go/cascade"
	"github.com/LumeraProtocol/sdk-go/types"
	"github.com/spf13/cobra"

	"lumera-ica-client/client"
)

// newUploadCmd registers the "upload" command and wires ICA-based registration.
// It resolves the ICA address, signs metadata with the controller key, and returns
// a JSON payload containing action/task IDs and both ICA addresses.
func newUploadCmd(app *app) *cobra.Command {
	var filePath string
	var actionID string
	var public bool
	cmd := &cobra.Command{
		Use:   "upload [file]",
		Short: "Upload file via ICA",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			// Resolve file path from flag/arg and load config.
			filePath, err = resolveOptionalArg(filePath, args, "file")
			if err != nil {
				return err
			}
			cfg, err := app.loadConfig()
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd)
			defer cancel()

			// Normalize to an absolute path so downstream logs/metadata are consistent.
			absPath, err := filepath.Abs(filePath)
			if err != nil {
				return err
			}
			// Create the SDK cascade client backed by the controller keyring.
			cascClient, err := client.NewCascadeClient(ctx, cfg)
			if err != nil {
				return err
			}
			defer cascClient.Cascade.Close()

			if strings.TrimSpace(actionID) != "" {
				bc, err := client.NewLumeraClient(ctx, cfg, cascClient.Keyring, cfg.Controller.KeyName)
				if err != nil {
					return err
				}
				defer bc.Close()

				action, err := bc.Action.GetAction(ctx, actionID)
				if err != nil {
					return err
				}
				if action.State != types.ActionStatePending {
					return fmt.Errorf("action %s state is %s; expected %s", action.ID, action.State, types.ActionStatePending)
				}

				signer := strings.TrimSpace(action.Creator)
				taskID, err := cascClient.Cascade.UploadToSupernode(ctx, action.ID, absPath, signer)
				if err != nil {
					return err
				}
				payload := map[string]any{
					"status":            "ok",
					"action_id":         action.ID,
					"tx_hash":           "",
					"task_id":           taskID,
					"ica_address":       action.Creator,
					"ica_owner_address": cascClient.OwnerAddress,
					"file":              absPath,
				}
				if meta, ok := action.Metadata.(*types.CascadeMetadata); ok && meta != nil {
					payload["is_public"] = meta.Public
				}
				return writeJSON(payload)
			}

			// Build a controller helper for ICA operations and resolve the ICA address.
			controller, err := client.NewICAController(ctx, cfg, cascClient.Keyring)
			if err != nil {
				return err
			}
			defer controller.Close()
			icaAddr, err := controller.EnsureICAAddress(ctx)
			if err != nil {
				return err
			}
			// Bridge the request into a controller-side ICA transaction.
			sendFunc := func(ctx context.Context, msg *actiontypes.MsgRequestAction, _ []byte, _ string, _ *cascade.UploadOptions) (*types.ActionResult, error) {
				return controller.SendRequestAction(ctx, msg)
			}
			// Build and submit the action registration with ICA creator + app pubkey.
			res, err := cascClient.Cascade.Upload(ctx, icaAddr, nil, absPath,
				cascade.WithICACreatorAddress(icaAddr),
				cascade.WithAppPubkey(controller.AppPubkey()),
				cascade.WithICASendFunc(sendFunc),
				cascade.WithPublic(public),
			)
			if err != nil {
				return err
			}
			return writeJSON(map[string]any{
				"status":            "ok",
				"action_id":         res.ActionID,
				"tx_hash":           res.TxHash,
				"task_id":           res.TaskID,
				"ica_address":       icaAddr,
				"ica_owner_address": controller.OwnerAddress(),
				"is_public":         public,
				"file":              absPath,
			})
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "Path to file to upload")
	cmd.Flags().StringVar(&actionID, "action-id", "", "Existing action ID to upload bytes for (skips action registration)")
	cmd.Flags().BoolVar(&public, "public", false, "Make uploaded file publicly accessible")
	return cmd
}
