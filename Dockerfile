FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETOS
ARG TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/fleetpkg-mcp /fleetpkg-mcp
ENTRYPOINT ["/fleetpkg-mcp"]
