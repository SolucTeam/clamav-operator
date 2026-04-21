# =============================================================================
# ClamAV Node Scanner — Standalone Image
# =============================================================================
# This image embeds:
#   • Node.js runtime + scanner application
#   • ClamAV binaries (clamscan, freshclam, sigtool)
#   • ClamAV signature databases (main.cvd, daily.cvd, bytecode.cvd)
#
# Modes:
#   SCAN_MODE=standalone  →  local clamscan, zero network dependency (default)
#   SCAN_MODE=remote      →  connects to external clamd (legacy)
#
# Air-gap usage:
#   The default build (DOWNLOAD_SIGS=true) runs freshclam during docker build
#   and bakes signatures into the image — requires internet at build time only.
#   At runtime set UPDATE_SIGNATURES=false so the container never reaches out.
#
#   For fully air-gapped builds (no internet even at build time):
#     1. On an internet-connected machine: mkdir sigs && freshclam --datadir=sigs
#     2. Copy the sigs/ directory into scanner/sigs/ (next to this Dockerfile)
#     3. docker build --build-arg DOWNLOAD_SIGS=false -t clamav-node-scanner:airgap .
#
# Build:
#   docker build -t clamav-node-scanner:latest .
#   docker build --build-arg DOWNLOAD_SIGS=false -t clamav-node-scanner:airgap .
# =============================================================================

# ─── Stage 1a: Install ClamAV binaries (stable layer, rarely invalidated) ─────
# Kept separate from the signature download stage to maximise Docker cache reuse:
# if the binaries have not changed, this layer is reused even when signatures
# are updated.
FROM debian:bookworm-slim AS clamav-base

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        clamav \
        clamav-freshclam \
        clamav-daemon \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# ─── Stage 1b: Download signatures (isolated layer, ~80% cache hit rate) ───────
# By isolating freshclam in its own stage, Docker can reuse this layer as long
# as the base image and build arguments have not changed.
#
# Two build modes:
#
#   DOWNLOAD_SIGS=true  (default)
#     freshclam runs during docker build — requires internet access at build time.
#     Use this for standard CI/CD pipelines that have outbound HTTPS access.
#
#   DOWNLOAD_SIGS=false  (air-gap)
#     freshclam is skipped. Pre-downloaded *.cvd / *.cld signature files must be
#     placed in the scanner/sigs/ directory (next to this Dockerfile) before
#     running docker build. See the air-gap workflow in the header above.
#
#     The build will FAIL if no signature files are found in /var/lib/clamav —
#     preventing a silently broken image from being pushed to the registry.
FROM clamav-base AS clamav-builder

ARG DOWNLOAD_SIGS=true

RUN mkdir -p /var/lib/clamav

# For air-gap builds (DOWNLOAD_SIGS=false), place pre-downloaded signature files
# in the scanner/sigs/ directory before running docker build. The directory is
# always COPY'd; it contains only a .gitkeep placeholder by default so this
# instruction never fails on a standard (connected) build.
COPY sigs/ /tmp/local-sigs/

RUN if [ "$DOWNLOAD_SIGS" = "true" ]; then \
        echo "Downloading ClamAV signatures via freshclam..." && \
        freshclam --datadir=/var/lib/clamav --stdout || true; \
    else \
        echo "DOWNLOAD_SIGS=false — copying pre-downloaded signatures from sigs/" && \
        find /tmp/local-sigs -maxdepth 1 \( -name '*.cvd' -o -name '*.cld' \) \
            -exec cp {} /var/lib/clamav/ \; ; \
    fi

# Fail the build if no usable signature database is present.
# This prevents a silently broken image from being pushed to the registry —
# a missing database only surfaces as a runtime error otherwise.
RUN ls -la /var/lib/clamav/ && \
    (test -f /var/lib/clamav/main.cvd || test -f /var/lib/clamav/main.cld) || \
    { echo ""; \
      echo "ERROR: No ClamAV main signature database found in /var/lib/clamav."; \
      echo ""; \
      echo "Fix options:"; \
      echo "  1. Build with DOWNLOAD_SIGS=true (requires internet at build time):"; \
      echo "       docker build -t clamav-node-scanner:latest ."; \
      echo "  2. Supply pre-downloaded signatures for air-gap builds:"; \
      echo "       mkdir scanner/sigs && freshclam --datadir=scanner/sigs"; \
      echo "       docker build --build-arg DOWNLOAD_SIGS=false \\"; \
      echo "                    -t clamav-node-scanner:airgap ."; \
      echo ""; \
      exit 1; \
    }

# ─── Stage 2: Node.js application ────────────────────────────────────────────
FROM node:20-bookworm-slim AS app-builder

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts
COPY src/ src/

# ─── Stage 3: Final runtime image ────────────────────────────────────────────
FROM node:20-bookworm-slim

LABEL maintainer="SolucTeam <contact@solucteam.com>"
LABEL description="ClamAV standalone node scanner for Kubernetes"
LABEL org.opencontainers.image.source="https://github.com/SolucTeam/clamav-operator"

# Install ClamAV runtime dependencies (shared libs only, no daemon)
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        clamav \
        clamav-freshclam \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    # Ensure directories exist with proper permissions
    mkdir -p /var/lib/clamav /results /app && \
    # freshclam config: set DatabaseDirectory
    sed -i 's|^DatabaseMirror.*|DatabaseMirror database.clamav.net|' /etc/clamav/freshclam.conf 2>/dev/null || true

# Copy ClamAV signature databases from builder
COPY --from=clamav-builder /var/lib/clamav/ /var/lib/clamav/

# Copy Node.js application
WORKDIR /app
COPY --from=app-builder /app/node_modules ./node_modules
COPY --from=app-builder /app/package.json ./
COPY --from=app-builder /app/src ./src

# ─── Drop privileges — never run as root ─────────────────────────────────────
# node:20-bookworm-slim ships a "node" user (uid 1000). Give it ownership of
# the directories it needs to write to at runtime.
RUN chown -R node:node /app /results /var/lib/clamav
USER node

# ─── Default environment ─────────────────────────────────────────────────────
ENV NODE_NAME="unknown" \
    HOST_ROOT="/host" \
    RESULTS_DIR="/results" \
    SCAN_MODE="standalone" \
    CLAMSCAN_PATH="/usr/bin/clamscan" \
    CLAMAV_DB_PATH="/var/lib/clamav" \
    UPDATE_SIGNATURES="false" \
    PATHS_TO_SCAN="/host/var/lib,/host/opt" \
    MAX_CONCURRENT="5" \
    FILE_TIMEOUT="300000" \
    MAX_FILE_SIZE="104857600" \
    INCREMENTAL_ENABLED="false" \
    SCAN_STRATEGY="full"

# ─── Healthcheck — verify clamscan is functional ─────────────────────────────
HEALTHCHECK --interval=60s --timeout=10s --retries=2 \
    CMD clamscan --version || exit 1

ENTRYPOINT ["node", "src/index.js"]
