package csr

import (
	"context"
	"testing"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApproverShouldApprove(t *testing.T) {
	approver := NewApprover(fake.NewSimpleClientset(), nil)
	approver.maxAge = 24 * time.Hour

	tests := []struct {
		name     string
		request  certificatesv1.CertificateSigningRequest
		expected bool
	}{
		{
			name: "approved signer and fresh request",
			request: certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "valid-csr",
					CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				},
			},
			expected: true,
		},
		{
			name: "kubelet-serving signer is allowed",
			request: certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "serving-csr",
					CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kubelet-serving",
				},
			},
			expected: true,
		},
		{
			name: "disallowed signer",
			request: certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "blocked-csr",
					CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "example.com/other",
				},
			},
			expected: false,
		},
		{
			name: "already approved",
			request: certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "already-approved",
					CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{{
						Type:   certificatesv1.CertificateApproved,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := approver.shouldApprove(tc.request); got != tc.expected {
				t.Fatalf("shouldApprove() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestApproverRunOnce(t *testing.T) {
	client := fake.NewSimpleClientset(
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pending-csr",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
			},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
			},
		},
	)

	approver := NewApprover(client, nil)
	if err := approver.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() returned unexpected error: %v", err)
	}

	csr, err := client.CertificatesV1().CertificateSigningRequests().Get(context.Background(), "pending-csr", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() returned unexpected error: %v", err)
	}

	if len(csr.Status.Conditions) == 0 {
		t.Fatal("expected approved condition to be set")
	}

	if csr.Status.Conditions[0].Type != certificatesv1.CertificateApproved {
		t.Fatalf("expected approval type %q, got %q", certificatesv1.CertificateApproved, csr.Status.Conditions[0].Type)
	}
}

func TestApproverRunRecoveryLoop(t *testing.T) {
	client := fake.NewSimpleClientset(
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "bootstrap-csr",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-30 * time.Minute)),
			},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: bootstrapSignerName,
			},
		},
	)

	approver := NewApprover(client, nil)
	approver.monitorTick = 10 * time.Millisecond
	approver.approvalDelay = 20 * time.Millisecond
	approver.recoveryTime = 50 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := approver.RunRecoveryLoop(ctx); err != nil {
		t.Fatalf("RunRecoveryLoop() returned unexpected error: %v", err)
	}

	requests, err := client.CertificatesV1().CertificateSigningRequests().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List() returned unexpected error: %v", err)
	}

	approvedCount := 0
	for i := range requests.Items {
		for _, condition := range requests.Items[i].Status.Conditions {
			if condition.Type == certificatesv1.CertificateApproved && condition.Status == corev1.ConditionTrue {
				approvedCount++
			}
		}
	}

	if approvedCount == 0 {
		t.Fatal("expected a bootstrap CSR to be approved during the recovery window")
	}
}
