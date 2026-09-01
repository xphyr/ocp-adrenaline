# ocp-adrenaline

A Red Hat CoreOS-friendly OpenShift CSR recovery helper.

## Summary

This project watches pending CertificateSigningRequests during startup and automatically approves the minimum set needed to recover a cluster after a prolonged powered-off state. It first approves the bootstrap signer requests, then waits for five minutes before approving kubelet-serving requests if the cluster still has not recovered.

## Correct RHCOS deployment model

This tool is intended for OpenShift / RHCOS and should be deployed using the CoreOS layering process documented by Red Hat, not a direct file injection via a normal `MachineConfig`.

The supported pattern is:

1. Build a custom layered RHCOS image from the cluster base image using a Containerfile.
2. Apply that image through a `MachineOSConfig` custom resource.
3. Let the Machine Config Operator roll the image out to the target machine config pool.

This is the approach described in the OpenShift Machine Configuration documentation for custom layered images.

## Build the binary

go build -o ocp-adrenaline .

## Build a custom layered image

The Containerfile in this repository is written as a custom layered image for RHCOS. Use the cluster base image digest from `oc adm release info --image-for rhel-coreos` and build with:

```bash
docker build -f Containerfile -t ocp-adrenaline:latest .
```

## Required OpenShift objects

Use the example in [machineconfig.yaml](machineconfig.yaml) as a `MachineOSConfig` object, not as a `MachineConfig` that writes files directly onto the host.

The example is meant to be applied through the MCO flow and then associated with a machine config pool that should receive the layered image.

## Example systemd unit

See [ocp-adrenaline.service](ocp-adrenaline.service).
