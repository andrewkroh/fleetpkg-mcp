FROM alpine:3.23
ARG TARGETOS
ARG TARGETARCH

# Install git and required packages
RUN apk add --no-cache git ca-certificates tzdata

# Create non-root user matching distroless nonroot UID
RUN adduser -D -u 65532 -h /home/fleetpkg fleetpkg

# Create data directory for integrations repo
RUN mkdir -p /data && chown -R fleetpkg:fleetpkg /data

# Copy binary
COPY ${TARGETOS}/${TARGETARCH}/fleetpkg-mcp /fleetpkg-mcp
RUN chmod +x /fleetpkg-mcp

# Switch to non-root user
USER fleetpkg
WORKDIR /data

# Set default refresh interval to 24 hours
ENV FLEETPKG_MCP_REFRESH_INTERVAL=24h

# Expose HTTP port
EXPOSE 8080

# Binary as ENTRYPOINT, default args in CMD
ENTRYPOINT ["/fleetpkg-mcp"]
CMD ["-dir", "/data/integrations", "-git-pull", "-http", "0.0.0.0:8080"]
