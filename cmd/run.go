package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ocp-adrenaline/pkg/csr"
	"ocp-adrenaline/pkg/k8s"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/spf13/cobra"
)

var kubeconfigPath string

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Scan for and approve the minimum pending kubelet CSRs needed to start the cluster.",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if _, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
			logger.Debug("systemd readiness notification unavailable", "error", err)
		}

		client, err := k8s.NewClient(kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to initialize Kubernetes client: %w", err)
		}

		agent := csr.NewApprover(client, logger)
		monitorCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := agent.RunRecoveryLoop(monitorCtx); err != nil {
			return err
		}

		logger.Info("startup CSR recovery window complete")
		return nil
	},
}

func init() {
	runCmd.Flags().StringVar(&kubeconfigPath, "kubeconfig", "", "path to kubeconfig file; empty uses in-cluster config or ~/.kube/config")
}
