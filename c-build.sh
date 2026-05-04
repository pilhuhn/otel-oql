#!/bin/bash

# Build architecture-specific images
podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:aarch64 --platform linux/aarch64 .
podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:amd64 --platform linux/amd64 .

# Push architecture-specific images
podman push quay.io/pilhuhn/otel-oql:aarch64
podman push quay.io/pilhuhn/otel-oql:amd64

# Remove existing manifest if it exists locally (handles pull/replace scenario)
podman manifest rm quay.io/pilhuhn/otel-oql:latest || true

# Create new manifest
podman manifest create quay.io/pilhuhn/otel-oql:latest

# Add architecture-specific images to manifest
podman manifest add quay.io/pilhuhn/otel-oql:latest quay.io/pilhuhn/otel-oql:aarch64
podman manifest add quay.io/pilhuhn/otel-oql:latest quay.io/pilhuhn/otel-oql:amd64

# Push the multi-arch manifest
podman manifest push --all quay.io/pilhuhn/otel-oql:latest
