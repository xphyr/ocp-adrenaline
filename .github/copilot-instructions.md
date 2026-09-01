# Go Project Instructions and Standards

## Project Context
- **Language**: Go (Golang)
- **Minimum Go Version**: 1.24+
- **Project Type**: CLI tool running as a systemd service (Kubernetes Certificate Signing Request automated approver).
- **Primary Libraries**: `kubernetes/client-go`, `spf13/cobra`, `coreos/go-systemd`.

## 1. General Principles & Code Style
- **Idiomatic Go**: Write clean, idiomatic code adhering to "Effective Go" principles.
- **Nesting & Return Early**: Minimize code nesting. Handle errors and edge cases early, returning from the function immediately to keep the happy path left-aligned.
- **Naming Conventions**:
  - Packages: Use single-word, lowercase names (e.g., `cmd`, `csr`, `k8s`). Avoid underscores or mixedCaps.
  - Variables/Functions: Use `mixedCaps` or `MixedCaps`.
  - Acronyms: Keep acronyms in consistent casing (e.g., `csrName`, `k8sClient`, `apiURL`).

## 2. Project Architecture & Layout
- **CLI Layout**: Follow a standard structure for CLI tools:
  - `/cmd/`: Entry points and Cobra commands (e.g., `/cmd/root.go`, `/cmd/run.go`).
  - `/pkg/csr/`: Domain logic for inspecting, validating, and approving CertificateSigningRequests.
  - `/pkg/k8s/`: Kubernetes client initialization, authentication handling (in-cluster vs. kubeconfig).
  - `/main.go`: Call `cmd.Execute()`. Keep it strictly minimal.
- **Dependency Injection**: Pass initialized `kubernetes.Interface` or customized interfaces into structs via constructors (e.g., `NewApprover(client kubernetes.Interface)`). Avoid global clients or state.

## 3. Kubernetes & Client-Go Standards
- **Informers & Watchers**: Prefer using `SharedInformerFactory` and caching mechanisms over tight polling loops to watch `CertificateSigningRequests` resources efficiently.
- **API Operations**: Always pass down the active `context.Context` to `client-go` API operations (e.g., `clientset.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, ...)`).
- **Fallback Authentication**: Support both in-cluster configuration (`rest.InClusterConfig()`) for running inside Kubernetes, and local kubeconfig parsing (`clientcmd.BuildConfigFromFlags`) for local testing.

## 4. Systemd & Linux Integration
- **Signal Handling**: Implement robust OS signal notification handling (`os.Interrupt`, `syscall.SIGTERM`) using `signal.NotifyContext` to guarantee graceful shutdown.
- **Systemd Watchdog & Readiness**: Utilize `coreos/go-systemd/v22/daemon` to notify systemd when the service is ready (`daemon.SdNotify(false, daemon.SdNotifyReady)`) or to pet the systemd watchdog if enabled.
- **Structured Logging**: Write all log messages to `os.Stdout` or `os.Stderr` using `log/slog`. Systemd (Journald) will automatically capture and index these standard outputs. Do not manage log files manually.

## 5. Error Handling & Safety
- **No Panics**: Do not use `panic` under normal operations. Return errors gracefully, wrap them with context using `fmt.Errorf("approver error: %w", err)`.
- **Approval Constraints**: Ensure thorough boundary conditions before issuing an approval command. The code must explicitly verify target certificate constraints (e.g., age thresholds, signer name validation) before triggering automation.

## 6. Testing Standards
- **Kubernetes Fake Clients**: Use `k8s.io/client-go/kubernetes/fake` to create unit tests for the controller loop logic without hitting a real API.
- **Table-Driven Tests**: Construct test layouts utilizing the standard table-driven testing pattern.
- **Resource Cleanup**: Ensure `defer` is leveraged immediately after context initialization or client instantiation to avoid resource leaks.

