# Runtime image for the Ponta Drive API.
#
# The API talks to S3-compatible storage (AWS S3, Cloudflare R2, MinIO) over
# HTTPS, so the runtime must ship a CA bundle. debian:stable-slim does not
# include one, which made every outbound TLS call fail with:
#
#   tls: failed to verify certificate: x509: certificate signed by unknown authority
#
# See docker-compose.yml, which builds this image and bind-mounts the repo.
FROM debian:stable-slim

# ca-certificates: verify the storage endpoint's TLS chain. Without it every
#   HTTPS call to the cloud provider fails with an unknown-authority error.
# procps: provides `pgrep`, which the compose healthcheck uses.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates procps \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
ENTRYPOINT ["/app/main"]
