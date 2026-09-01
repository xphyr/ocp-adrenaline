package csr

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultMaxAge        = 24 * time.Hour
	defaultApprovalDelay = 5 * time.Minute
	defaultRecoveryTime  = 10 * time.Minute
	defaultMonitorTick   = 30 * time.Second
	bootstrapSignerName  = "kubernetes.io/kube-apiserver-client-kubelet"
	servingSignerName    = "kubernetes.io/kubelet-serving"
)

var defaultAllowedSigners = map[string]struct{}{
	bootstrapSignerName: {},
	servingSignerName:   {},
}

type Approver struct {
	client         kubernetes.Interface
	logger         *slog.Logger
	maxAge         time.Duration
	approvalDelay  time.Duration
	recoveryTime   time.Duration
	monitorTick    time.Duration
	allowedSigners map[string]struct{}
}

func NewApprover(client kubernetes.Interface, logger *slog.Logger) *Approver {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	return &Approver{
		client:         client,
		logger:         logger,
		maxAge:         defaultMaxAge,
		approvalDelay:  defaultApprovalDelay,
		recoveryTime:   defaultRecoveryTime,
		monitorTick:    defaultMonitorTick,
		allowedSigners: defaultAllowedSigners,
	}
}

func (a *Approver) HealthCheck() bool {
	return a != nil && a.client != nil
}

func (a *Approver) RunOnce(ctx context.Context) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("approver error: client is nil")
	}

	approvedCount, err := a.approvePending(ctx, bootstrapSignerName, servingSignerName)
	if err != nil {
		return err
	}

	if approvedCount == 0 {
		a.logger.Debug("no pending kubelet CSRs required approval")
	}

	return nil
}

func (a *Approver) RunStartupSequence(ctx context.Context) error {
	return a.RunRecoveryLoop(ctx)
}

func (a *Approver) RunRecoveryLoop(ctx context.Context) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("approver error: client is nil")
	}

	if a.monitorTick <= 0 {
		a.monitorTick = defaultMonitorTick
	}
	if a.recoveryTime <= 0 {
		a.recoveryTime = defaultRecoveryTime
	}
	if a.approvalDelay <= 0 {
		a.approvalDelay = defaultApprovalDelay
	}

	deadline := time.Now().Add(a.recoveryTime)
	for {
		if time.Now().After(deadline) || time.Now().Equal(deadline) {
			return nil
		}

		if pending, err := a.hasPendingSigner(ctx, bootstrapSignerName, servingSignerName); err != nil {
			return err
		} else if !pending {
			a.logger.Debug("no pending CSRs requiring approval during recovery window")
			return nil
		}

		elapsed := time.Since(deadline.Add(-a.recoveryTime))
		if elapsed < a.approvalDelay {
			if _, err := a.approvePending(ctx, bootstrapSignerName); err != nil {
				return err
			}
		} else {
			if _, err := a.approvePending(ctx, servingSignerName); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			a.logger.Info("recovery window interrupted; exiting cleanly")
			return nil
		case <-time.After(a.monitorTick):
		}
	}
}

func (a *Approver) approvePending(ctx context.Context, signerNames ...string) (int, error) {
	requests, err := a.client.CertificatesV1().CertificateSigningRequests().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("approver error: %w", err)
	}

	approvedCount := 0
	for i := range requests.Items {
		item := requests.Items[i]
		if !a.shouldApprove(item) {
			continue
		}
		if !matchesSigner(item.Spec.SignerName, signerNames...) {
			continue
		}

		approved := item.DeepCopy()
		approved.Status.Conditions = append(approved.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateApproved,
			Status:  corev1.ConditionTrue,
			Reason:  "AutoApproved",
			Message: "approved by ocp-adrenaline",
		})

		if _, err = a.client.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, item.Name, approved, metav1.UpdateOptions{}); err != nil {
			return 0, fmt.Errorf("approver error: %w", err)
		}

		approvedCount++
		a.logger.Info("approved certificate signing request", "csr_name", item.Name, "signer_name", item.Spec.SignerName)
	}

	return approvedCount, nil
}

func (a *Approver) hasPendingSigner(ctx context.Context, signerNames ...string) (bool, error) {
	requests, err := a.client.CertificatesV1().CertificateSigningRequests().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("approver error: %w", err)
	}

	for i := range requests.Items {
		request := requests.Items[i]
		if matchesSigner(request.Spec.SignerName, signerNames...) && a.shouldApprove(request) {
			return true, nil
		}
	}

	return false, nil
}

func matchesSigner(actual string, expected ...string) bool {
	for _, signer := range expected {
		if actual == signer {
			return true
		}
	}
	return false
}

func (a *Approver) shouldApprove(request certificatesv1.CertificateSigningRequest) bool {
	if request.Name == "" {
		return false
	}

	if request.Spec.SignerName == "" {
		return false
	}

	if _, ok := a.allowedSigners[request.Spec.SignerName]; !ok {
		return false
	}

	if request.CreationTimestamp.IsZero() {
		return false
	}

	if time.Since(request.CreationTimestamp.Time) > a.maxAge {
		return false
	}

	for _, condition := range request.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved && condition.Status == corev1.ConditionTrue {
			return false
		}
	}

	return true
}
