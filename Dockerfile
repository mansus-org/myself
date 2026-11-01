# myself serve: the landing page at /, the REST API at /api/v1 (SPEC.md
# section 10). The brain lives on the volume at /data (MYSELF_FILE).
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w \
        -X github.com/mansus-org/myself/internal/cli.Version=$VERSION \
        -X github.com/mansus-org/myself/internal/cli.Commit=$COMMIT \
        -X github.com/mansus-org/myself/internal/cli.BuildDate=$BUILD_DATE" \
      -o /out/myself ./cmd/myself

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/myself /usr/local/bin/myself
# The landing page is embedded in the binary; this copy is for --site.
COPY site /site
USER 10001:10001
ENV MYSELF_FILE=/data/brain.jsonl
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/myself"]
CMD ["serve", "--addr", ":8080"]
