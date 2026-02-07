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
#   Build with --build-arg DOWNLOAD_SIGS=true (default) to bake signatures
#   into the image.  Set UPDATE_SIGNATURES=false at runtime so the container
#   never tries to reach the internet.
#
# Build:
#   docker build -t clamav-node-scanner:latest .
#   docker build --build-arg DOWNLOAD_SIGS=false -t clamav-node-scanner:airgap .
# =============================================================================

# ─── Stage 1: Install ClamAV binaries + optionally download signatures ───────
FROM debian:bookworm-slim AS clamav-builder

ARG DOWNLOAD_SIGS=true

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        clamav \
        clamav-freshclam \
        clamav-daemon \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Download initial signature databases (skipped when DOWNLOAD_SIGS=false for
# air-gap builds where signatures are injected by CI/CD)
RUN mkdir -p /var/lib/clamav && \
    if [ "$DOWNLOAD_SIGS" = "true" ]; then \
        freshclam --datadir=/var/lib/clamav --stdout || true; \
    fi

# Verify we have at least one signature file
RUN ls -la /var/lib/clamav/ && \
    (test -f /var/lib/clamav/main.cvd || test -f /var/lib/clamav/main.cld || \
     echo "WARNING: no main signature database found")

# ─── Stage 2: Node.js application ────────────────────────────────────────────
FROM node:25-bookworm-slim AS app-builder

WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --omit=dev --ignore-scripts 2>/dev/null || npm install --omit=dev --ignore-scripts
COPY src/ src/

# ─── Stage 3: Final runtime image ────────────────────────────────────────────
FROM node:25-bookworm-slim

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
