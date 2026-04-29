
podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:aarch64 --platform linux/aarch64 .
podman build -f container/Containerfile -t quay.io/pilhuhn/otel-oql:amd64 --platform linux/amd64 .

podman push quay.io/pilhuhn/otel-oql:aarch64
podman push quay.io/pilhuhn/otel-oql:amd64

