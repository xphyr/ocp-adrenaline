# This file builds a custom layered RHCOS image for OpenShift.
# Replace the digest below with the exact base image used by your cluster:
#   oc adm release info --image-for rhel-coreos
FROM quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:<replace-with-rhcos-base-image-digest>

COPY ./ocp-adrenaline /usr/local/bin/ocp-adrenaline
COPY ./ocp-adrenaline.service /etc/systemd/system/ocp-adrenaline.service

RUN chmod 0755 /usr/local/bin/ocp-adrenaline && \
    chmod 0644 /etc/systemd/system/ocp-adrenaline.service && \
    systemctl enable ocp-adrenaline.service && \
    bootc container lint

# The service is enabled as part of the layered image, and the MCO applies it to the
# target MachineConfigPool as part of the custom layered-image rollout.
