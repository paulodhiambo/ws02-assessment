# Static Go binary on an empty base image (~8 MB instead of ~230 MB with Node).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /customer-service .

FROM scratch
COPY --from=build /customer-service /customer-service
USER 65534:65534
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/customer-service", "-healthcheck"]
ENTRYPOINT ["/customer-service"]
