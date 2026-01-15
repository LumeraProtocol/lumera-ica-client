package commands

import (
	"encoding/base64"
	"strings"

	"github.com/LumeraProtocol/sdk-go/cascade"
	"github.com/LumeraProtocol/sdk-go/types"
	"github.com/spf13/cobra"

	"lumera-ica-client/client"
)

// newActionCmd groups action-related subcommands.
func newActionCmd(app *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "action",
		Short: "Action management commands",
	}
	cmd.AddCommand(newActionApproveCmd(app))
	cmd.AddCommand(newActionStatusCmd(app))
	return cmd
}

// newActionApproveCmd approves an action via ICA, optionally resolving the ICA address.
func newActionApproveCmd(app *app) *cobra.Command {
	var actionID string
	var icaAddress string
	cmd := &cobra.Command{
		Use:   "approve [action-id]",
		Short: "Approve an action via ICA",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			// Resolve input and load config for the controller chain.
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

			// Initialize cascade client + controller helper.
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
			// Resolve ICA address from the controller key if not provided.
			if strings.TrimSpace(icaAddress) == "" {
				icaAddress, err = controller.ICAAddress(ctx)
				if err != nil {
					return err
				}
			}
			// Build and submit the approve action message through ICA.
			msg, err := cascade.CreateApproveActionMessage(ctx, actionID, cascade.WithApproveCreator(icaAddress))
			if err != nil {
				return err
			}
			txHash, err := controller.SendApproveAction(ctx, msg)
			if err != nil {
				return err
			}
			return writeJSON(map[string]any{
				"status":            "ok",
				"action_id":         actionID,
				"tx_hash":           txHash,
				"ica_address":       icaAddress,
				"ica_owner_address": controller.OwnerAddress(),
			})
		},
	}
	cmd.Flags().StringVar(&actionID, "action-id", "", "Action ID to approve")
	cmd.Flags().StringVar(&icaAddress, "ica-address", "", "ICA address to approve from")
	return cmd
}

// newActionStatusCmd queries the action module and returns an enriched JSON payload.
func newActionStatusCmd(app *app) *cobra.Command {
	var actionID string
	cmd := &cobra.Command{
		Use:   "status [action-id]",
		Short: "Get action status by ID",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			// Resolve input and load config for the Lumera chain.
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

			// Reuse the controller keyring and construct a Lumera gRPC client.
			cascClient, err := client.NewCascadeClient(ctx, cfg)
			if err != nil {
				return err
			}
			defer cascClient.Cascade.Close()

			bc, err := client.NewLumeraClient(ctx, cfg, cascClient.Keyring, cfg.Controller.KeyName)
			if err != nil {
				return err
			}
			defer bc.Close()

			// Fetch the action and emit derived fields such as app_pubkey and is_public.
			action, err := bc.Action.GetAction(ctx, actionID)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"status":       "ok",
				"action_id":    action.ID,
				"state":        action.State,
				"type":         action.Type,
				"creator":      action.Creator,
				"price":        action.Price,
				"block_height": action.BlockHeight,
				"expires_at":   action.ExpirationTime.Unix(),
			}
			if meta, ok := action.Metadata.(*types.CascadeMetadata); ok && meta != nil {
				payload["is_public"] = meta.Public
			}
			if len(action.AppPubkey) > 0 {
				payload["app_pubkey"] = base64.StdEncoding.EncodeToString(action.AppPubkey)
			}
			return writeJSON(payload)
		},
	}
	cmd.Flags().StringVar(&actionID, "action-id", "", "Action ID to query")
	return cmd
}
